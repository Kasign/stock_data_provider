package httpclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/observability"
)

var (
	errStreamIdleTimeout  = errors.New("stream idle read timeout")
	errStreamBodyTooLarge = errors.New("stream body exceeds configured limit")
)

// StreamStats 记录单条流的安全统计信息，不包含 URL、凭据或响应正文。
type StreamStats struct {
	RequestID         string
	Dependency        string
	StatusCode        int
	StartedAt         time.Time
	Duration          time.Duration
	BytesRead         int64
	TerminationReason string
}

// StreamResponse 在响应头到达后立即返回。Body 的所有权交给调用方，调用方必须负责关闭。
type StreamResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
	state      *streamState
}

// Stats 返回当前流的统计快照；流终止后 Duration 和终止原因固定。
func (r *StreamResponse) Stats() StreamStats {
	if r == nil || r.state == nil {
		return StreamStats{}
	}
	return r.state.snapshot()
}

type streamState struct {
	deliveryMu sync.Mutex
	mu         sync.Mutex
	cleanup    sync.Once

	closed      bool
	terminalErr error
	closeErr    error
	cancel      context.CancelFunc
	stopContext func() bool
	release     func()
	body        io.ReadCloser
	stats       StreamStats
}

func (s *streamState) setStop(stop func() bool) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		stop()
		return
	}
	s.stopContext = stop
	s.mu.Unlock()
}

func (s *streamState) snapshot() StreamStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.stats
	if !s.closed {
		result.Duration = time.Since(result.StartedAt)
	}
	return result
}

func (s *streamState) isClosed() (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed, s.terminalErr
}

func (s *streamState) addBytes(count int) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.stats.BytesRead += int64(count)
	return true
}

func (s *streamState) finish(reason string, terminalErr error) error {
	s.deliveryMu.Lock()
	defer s.deliveryMu.Unlock()
	return s.finishLocked(reason, terminalErr)
}

// finishLocked 只能在持有 deliveryMu 时调用，用于串行化终止事件与 Read 的最终数据交付。
func (s *streamState) finishLocked(reason string, terminalErr error) error {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		s.terminalErr = terminalErr
		s.stats.TerminationReason = reason
		s.stats.Duration = time.Since(s.stats.StartedAt)
	}
	s.mu.Unlock()

	s.cleanup.Do(func() {
		s.mu.Lock()
		stop := s.stopContext
		s.mu.Unlock()
		if stop != nil {
			stop()
		}
		if s.cancel != nil {
			s.cancel()
		}
		if s.body != nil {
			s.closeErr = s.body.Close()
		}
		if s.release != nil {
			s.release()
		}
	})
	return s.closeErr
}

type streamBody struct {
	source      io.ReadCloser
	state       *streamState
	ctx         context.Context
	idleTimeout time.Duration
	maxBytes    int64
	bytesRead   int64
}

func (b *streamBody) Read(buffer []byte) (int, error) {
	if len(buffer) == 0 {
		return 0, nil
	}
	if closed, terminalErr := b.state.isClosed(); closed {
		if terminalErr == nil {
			return 0, io.ErrClosedPipe
		}
		return 0, terminalErr
	}
	if err := b.ctx.Err(); err != nil {
		f := b.contextFault(err)
		reason := "context_cancel"
		if errors.Is(err, context.DeadlineExceeded) {
			reason = "deadline"
		}
		b.state.finish(reason, f)
		return 0, b.state.readTerminalError()
	}

	if b.bytesRead >= b.maxBytes {
		var probe [1]byte
		n, err := b.readSource(probe[:])
		if n > 0 {
			f := b.bodyLimitFault()
			b.state.finish("body_limit", f)
			return 0, b.state.readTerminalError()
		}
		if err == nil {
			return 0, nil
		}
		if errors.Is(err, io.EOF) {
			b.state.finish("eof", io.EOF)
			return 0, b.state.readTerminalError()
		}
		return 0, b.finishReadError(err)
	}

	allowed := int64(len(buffer))
	if allowed > b.maxBytes-b.bytesRead {
		allowed = b.maxBytes - b.bytesRead
	}
	n, err := b.readSource(buffer[:int(allowed)])

	b.state.deliveryMu.Lock()
	defer b.state.deliveryMu.Unlock()
	if closed, terminalErr := b.state.isClosed(); closed {
		if terminalErr == nil {
			return 0, io.ErrClosedPipe
		}
		return 0, terminalErr
	}
	if n > 0 {
		if !b.state.addBytes(n) {
			return 0, b.state.readTerminalError()
		}
		b.bytesRead += int64(n)
	}
	if err == nil {
		return n, nil
	}
	if errors.Is(err, io.EOF) {
		b.state.finishLocked("eof", io.EOF)
		return n, io.EOF
	}
	readFault, reason := b.readFault(err)
	b.state.finishLocked(reason, readFault)
	return n, b.state.readTerminalError()
}

func (s *streamState) readTerminalError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.terminalErr != nil {
		return s.terminalErr
	}
	return io.ErrClosedPipe
}

func (b *streamBody) readSource(buffer []byte) (int, error) {
	if b.idleTimeout <= 0 {
		return b.source.Read(buffer)
	}
	var timedOut atomic.Bool
	timer := time.AfterFunc(b.idleTimeout, func() {
		timedOut.Store(true)
		b.state.finish("idle_timeout", b.idleFault())
	})
	n, err := b.source.Read(buffer)
	timer.Stop()
	if timedOut.Load() {
		return n, errStreamIdleTimeout
	}
	return n, err
}

func (b *streamBody) Close() error {
	return b.state.finish("caller_close", io.ErrClosedPipe)
}

func (b *streamBody) contextFault(err error) *fault.Fault {
	if errors.Is(err, context.DeadlineExceeded) {
		return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_deadline_exceeded", "HTTP stream deadline exceeded", err, b.state.stats.Dependency, false)
	}
	return newStreamFault(fault.ClassUnavailable, "platform.httpclient.stream_canceled", "HTTP stream was canceled", err, b.state.stats.Dependency, false)
}

func (b *streamBody) idleFault() *fault.Fault {
	return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_idle_timeout", "HTTP stream read timed out while waiting for upstream data", errStreamIdleTimeout, b.state.stats.Dependency, false)
}

func (b *streamBody) bodyLimitFault() *fault.Fault {
	return newStreamFault(fault.ClassValidation, "platform.httpclient.stream_body_too_large", "HTTP stream body exceeds configured limit", errStreamBodyTooLarge, b.state.stats.Dependency, false)
}

func (b *streamBody) finishReadError(err error) error {
	if errors.Is(err, errStreamIdleTimeout) {
		b.state.finish("idle_timeout", b.idleFault())
		return b.state.readTerminalError()
	}
	readFault, reason := b.readFault(err)
	b.state.finish(reason, readFault)
	return b.state.readTerminalError()
}

func (b *streamBody) readFault(err error) (*fault.Fault, string) {
	if ctxErr := b.ctx.Err(); ctxErr != nil {
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			return b.contextFault(ctxErr), "deadline"
		}
		return b.contextFault(ctxErr), "context_cancel"
	}
	if errors.Is(err, errStreamIdleTimeout) {
		return b.idleFault(), "idle_timeout"
	}
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return newStreamFault(fault.ClassUnavailable, "platform.httpclient.stream_upstream_truncated", "upstream closed the HTTP stream before completion", err, b.state.stats.Dependency, false), "upstream_truncated"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_read_timeout", "HTTP stream read timed out", err, b.state.stats.Dependency, false), "read_timeout"
	}
	return newStreamFault(fault.ClassUnavailable, "platform.httpclient.stream_read_failed", "HTTP stream read failed", err, b.state.stats.Dependency, false), "read_error"
}

func newStreamFault(class fault.Class, code, message string, cause error, dependency string, retryable bool) *fault.Fault {
	return &fault.Fault{
		Class: class, Code: code, SafeMessage: message,
		Dependency: dependency, Retryable: retryable, Cause: cause,
	}
}

// OpenStream 受控地发起流式请求，不缓冲完整响应体。调用方拥有 Body，并必须在消费结束后关闭；StreamTimeout 限制整个流生命周期。
func (c *Client) OpenStream(ctx context.Context, request *http.Request, policy DestinationPolicy) (*StreamResponse, error) {
	if c == nil || ctx == nil || request == nil || request.URL == nil {
		return nil, fault.New(fault.ClassValidation, "platform.httpclient.invalid_request", "invalid HTTP request", nil)
	}
	if !validDependency(policy.Dependency) {
		return nil, fault.New(fault.ClassValidation, "platform.httpclient.invalid_dependency", "invalid dependency identifier", nil)
	}
	policy.AllowedHosts = append([]string(nil), policy.AllowedHosts...)
	policy.AllowedPorts = append([]uint16(nil), policy.AllowedPorts...)
	if err := validatePolicy(policy); err != nil {
		return nil, c.faultFor(err, policy.Dependency)
	}
	if (request.Host != "" && !strings.EqualFold(request.Host, request.URL.Host)) || hasHeaderFold(request.Header, "Host") {
		return nil, c.newFault(fault.ClassValidation, "platform.httpclient.custom_host_rejected", "custom Host header is not allowed", errPolicyRejected, policy.Dependency, false)
	}
	if hasHeaderFold(request.Header, "Accept-Encoding") {
		return nil, c.newFault(fault.ClassValidation, "platform.httpclient.accept_encoding_rejected", "custom Accept-Encoding header is not allowed", errPolicyRejected, policy.Dependency, false)
	}
	if err := validateURL(request.URL, policy); err != nil {
		return nil, c.faultFor(err, policy.Dependency)
	}

	startedAt := time.Now()
	requestCtx, cancel := context.WithTimeout(ctx, c.config.StreamTimeout)
	requestCtx = context.WithValue(requestCtx, policyContextKey{}, policy)
	// 先申请 Dependency 额度，再申请全局额度。等待某个 Dependency
	// 的请求不会占住全局槽位，避免阻塞其他可用 Dependency。
	dependencySemaphore := c.streamDependencySemaphore(policy.Dependency)
	if dependencySemaphore != nil {
		select {
		case dependencySemaphore <- struct{}{}:
		case <-requestCtx.Done():
			err := requestCtx.Err()
			cancel()
			return nil, c.streamContextFault(err, policy.Dependency)
		}
	}

	globalSemaphore := c.defaultStreamSemaphore
	select {
	case globalSemaphore <- struct{}{}:
	case <-requestCtx.Done():
		err := requestCtx.Err()
		if dependencySemaphore != nil {
			<-dependencySemaphore
		}
		cancel()
		return nil, c.streamContextFault(err, policy.Dependency)
	}
	release := func() {
		<-globalSemaphore
		if dependencySemaphore != nil {
			<-dependencySemaphore
		}
	}

	current := request.Clone(requestCtx)
	observability.PropagateRequestID(requestCtx, current)
	response, err := c.httpClient.Do(current)
	if err != nil {
		retryPolicy := policy
		retryPolicy.Idempotent = streamRequestRetryable(request, policy)
		streamFault := c.streamOpenFault(err, requestCtx, retryPolicy)
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		release()
		cancel()
		return nil, streamFault
	}
	if response.Body == nil {
		response.Body = http.NoBody
	}

	state := &streamState{
		cancel: cancel, release: release, body: response.Body,
		stats: StreamStats{
			RequestID:  observability.FromContext(requestCtx),
			Dependency: policy.Dependency,
			StatusCode: response.StatusCode,
			StartedAt:  startedAt,
		},
	}
	body := &streamBody{
		source: response.Body, state: state, ctx: requestCtx,
		idleTimeout: c.config.StreamIdleTimeout,
		maxBytes:    c.config.MaxStreamBytes,
	}
	stop := context.AfterFunc(requestCtx, func() {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			state.finish("deadline", body.contextFault(context.DeadlineExceeded))
			return
		}
		state.finish("context_cancel", body.contextFault(context.Canceled))
	})
	state.setStop(stop)

	return &StreamResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Body:       body,
		state:      state,
	}, nil
}

func (c *Client) streamDependencySemaphore(dependency string) chan struct{} {
	return c.streamSemaphores[dependency]
}

func (c *Client) streamContextFault(err error, dependency string) *fault.Fault {
	if errors.Is(err, context.DeadlineExceeded) {
		return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_deadline_exceeded", "HTTP stream deadline exceeded", err, dependency, false)
	}
	return newStreamFault(fault.ClassUnavailable, "platform.httpclient.stream_canceled", "HTTP stream was canceled", err, dependency, false)
}

type streamOpenErrorKind uint8

const (
	streamOpenErrorOther streamOpenErrorKind = iota
	streamOpenErrorHeadersTooLarge
	streamOpenErrorTLSHandshakeTimeout
	streamOpenErrorResponseHeaderTimeout
)

// classifyStreamOpenError 将 Go net/http Transport 未公开类型化的少数错误文本
// 收敛在一个兼容层；其他错误优先通过 errors.As/Is 按类型判断。
func classifyStreamOpenError(err error) streamOpenErrorKind {
	if err == nil {
		return streamOpenErrorOther
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "response headers exceeded"):
		return streamOpenErrorHeadersTooLarge
	case strings.Contains(message, "tls handshake timeout"):
		return streamOpenErrorTLSHandshakeTimeout
	case strings.Contains(message, "timeout awaiting response headers"):
		return streamOpenErrorResponseHeaderTimeout
	default:
		return streamOpenErrorOther
	}
}

// streamRequestRetryable 判断流式请求是否具备被调用方重新发起的基本条件。
// 除显式幂等外，请求体必须为空或可通过 GetBody 重新创建。
func streamRequestRetryable(request *http.Request, policy DestinationPolicy) bool {
	if request == nil || !policy.Idempotent {
		return false
	}
	return request.Body == nil || request.Body == http.NoBody || request.GetBody != nil
}

func (c *Client) streamOpenFault(err error, ctx context.Context, policy DestinationPolicy) *fault.Fault {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return c.streamContextFault(ctxErr, policy.Dependency)
	}
	if classifyStreamOpenError(err) == streamOpenErrorHeadersTooLarge {
		return newStreamFault(fault.ClassValidation, "platform.httpclient.stream_headers_too_large", "HTTP stream response headers exceed configured limit", err, policy.Dependency, false)
	}
	switch {
	case errors.Is(err, errPolicyRejected), errors.Is(err, errRedirectRejected):
		return newStreamFault(fault.ClassValidation, "platform.httpclient.stream_destination_rejected", "HTTP stream destination is not allowed", err, policy.Dependency, false)
	case errors.Is(err, errDNSResolution):
		return newStreamFault(fault.ClassUnavailable, "platform.httpclient.stream_dns_failed", "HTTP stream destination DNS resolution failed", err, policy.Dependency, false)
	}
	retryable := policy.Idempotent && retryableTransportError(err)
	switch classifyStreamOpenError(err) {
	case streamOpenErrorTLSHandshakeTimeout:
		return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_tls_handshake_timeout", "HTTP stream TLS handshake timed out", err, policy.Dependency, retryable)
	case streamOpenErrorResponseHeaderTimeout:
		return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_header_timeout", "HTTP stream response headers timed out", err, policy.Dependency, retryable)
	}
	var operationError *net.OpError
	if errors.As(err, &operationError) && operationError.Op == "dial" {
		if operationError.Timeout() || errors.Is(err, context.DeadlineExceeded) {
			return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_connect_timeout", "HTTP stream connection timed out", err, policy.Dependency, retryable)
		}
		return newStreamFault(fault.ClassUnavailable, "platform.httpclient.stream_connect_failed", "HTTP stream connection failed", err, policy.Dependency, retryable)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_connect_timeout", "HTTP stream connection timed out", err, policy.Dependency, retryable)
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return newStreamFault(fault.ClassTimeout, "platform.httpclient.stream_header_timeout", "HTTP stream response headers timed out", err, policy.Dependency, retryable)
	}
	return newStreamFault(fault.ClassUnavailable, "platform.httpclient.stream_connect_failed", "HTTP stream request failed before response headers", err, policy.Dependency, retryable)
}
