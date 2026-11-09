package httpclient

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"syscall"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/observability"
)

func newLocalStreamClient(t *testing.T, server *httptest.Server, maxBytes int64, idle time.Duration, maxStreams int) *Client {
	t.Helper()
	client, err := New(Config{
		Resolver:             staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		StreamTimeout:        3 * time.Second,
		StreamIdleTimeout:    idle,
		MaxStreamBytes:       maxBytes,
		MaxConcurrentStreams: maxStreams,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func openLocalStream(t *testing.T, client *Client, server *httptest.Server, ctx context.Context, path string, policy DestinationPolicy) *StreamResponse {
	t.Helper()
	response, err := client.OpenStream(ctx, localRequest(t, server, path), policy)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func TestOpenStreamDeliversFirstChunkBeforeEOF(t *testing.T) {
	firstWritten := make(chan struct{})
	allowFinish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		close(firstWritten)
		<-allowFinish
		_, _ = w.Write([]byte("second"))
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	response := openLocalStream(t, client, server, context.Background(), "/chunks", localPolicy(server))
	defer response.Body.Close()
	<-firstWritten
	buffer := make([]byte, 5)
	n, err := response.Body.Read(buffer)
	if err != nil || string(buffer[:n]) != "first" {
		t.Fatalf("expected first chunk before EOF, got %q, %v", buffer[:n], err)
	}
	select {
	case <-allowFinish:
		t.Fatal("server should still be waiting")
	default:
	}
	close(allowFinish)
}

func TestOpenStreamHTTP2IncrementalReadAndCancel(t *testing.T) {
	firstWritten := make(chan struct{})
	protocol := make(chan int, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		protocol <- r.ProtoMajor
		_, _ = w.Write([]byte("h2-first"))
		w.(http.Flusher).Flush()
		close(firstWritten)
		<-r.Context().Done()
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()

	client, err := New(Config{
		Resolver:             staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		StreamTimeout:        3 * time.Second,
		StreamIdleTimeout:    time.Second,
		MaxStreamBytes:       1024,
		MaxConcurrentStreams: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	serverTransport := server.Client().Transport.(*http.Transport)
	client.transport.TLSClientConfig = serverTransport.TLSClientConfig.Clone()
	// 测试服务证书包含 127.0.0.1 SAN；使用该名称校验 TLS，同时 HTTP Host 仍为 localhost。
	client.transport.TLSClientConfig.ServerName = "127.0.0.1"
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}

	port := server.Listener.Addr().(*net.TCPAddr).Port
	request, err := http.NewRequest(http.MethodGet, "https://localhost:"+strconv.Itoa(port)+"/h2", nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := DestinationPolicy{
		Dependency: "llm.stream", AllowedHosts: []string{"localhost"},
		AllowedPorts: []uint16{uint16(port)},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	response, err := client.OpenStream(ctx, request, policy)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	select {
	case major := <-protocol:
		if major != 2 {
			t.Fatalf("expected HTTP/2 request, got HTTP/%d", major)
		}
	case <-time.After(time.Second):
		t.Fatal("server did not receive HTTP/2 request")
	}
	<-firstWritten
	buffer := make([]byte, len("h2-first"))
	n, err := response.Body.Read(buffer)
	if err != nil || string(buffer[:n]) != "h2-first" {
		t.Fatalf("expected HTTP/2 first chunk before cancellation, got %q, %v", buffer[:n], err)
	}

	readResult := make(chan error, 1)
	go func() {
		var next [8]byte
		_, readErr := response.Body.Read(next[:])
		readResult <- readErr
	}()
	cancel()
	select {
	case readErr := <-readResult:
		var platformFault *fault.Fault
		if !errors.As(readErr, &platformFault) || platformFault.Code != "platform.httpclient.stream_canceled" {
			t.Fatalf("expected HTTP/2 canceled Fault, got %v", readErr)
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP/2 blocked Read did not stop after cancellation")
	}
}

func TestOpenStreamContextCancelInterruptsBlockedRead(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		close(ready)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	ctx, cancel := context.WithCancel(context.Background())
	response := openLocalStream(t, client, server, ctx, "/cancel", localPolicy(server))
	defer response.Body.Close()
	<-ready
	result := make(chan error, 1)
	go func() { var buffer [8]byte; _, err := response.Body.Read(buffer[:]); result <- err }()
	cancel()
	select {
	case err := <-result:
		var f *fault.Fault
		if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_canceled" {
			t.Fatalf("expected canceled Fault, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked Read did not stop after cancellation")
	}
}

func TestOpenStreamDeadlineInterruptsBlockedRead(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		close(ready)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	response := openLocalStream(t, client, server, ctx, "/deadline", localPolicy(server))
	defer response.Body.Close()
	<-ready
	var buffer [8]byte
	_, err := response.Body.Read(buffer[:])
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_deadline_exceeded" {
		t.Fatalf("expected deadline Fault, got %v", err)
	}
}

func TestOpenStreamCloseReleasesLifecycleSlot(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 1)
	first := openLocalStream(t, client, server, context.Background(), "/first", localPolicy(server))
	secondResult := make(chan *StreamResponse, 1)
	secondError := make(chan error, 1)
	go func() {
		response, err := client.OpenStream(context.Background(), localRequest(t, server, "/second"), localPolicy(server))
		if err != nil {
			secondError <- err
			return
		}
		secondResult <- response
	}()
	select {
	case <-secondResult:
		t.Fatal("second stream bypassed occupied slot")
	case err := <-secondError:
		t.Fatalf("second stream failed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	if err := first.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case second := <-secondResult:
		_ = second.Body.Close()
	case err := <-secondError:
		t.Fatal(err)
	case <-time.After(time.Second):
		t.Fatal("slot was not released after Close")
	}
}

func TestOpenStreamReportsUpstreamTruncation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	response := openLocalStream(t, client, server, context.Background(), "/truncated", localPolicy(server))
	defer response.Body.Close()
	_, err := io.ReadAll(response.Body)
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_upstream_truncated" {
		t.Fatalf("expected truncation Fault, got %v", err)
	}
}

func TestOpenStreamStopsAtCumulativeBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("12345")) }))
	defer server.Close()
	client := newLocalStreamClient(t, server, 4, time.Second, 2)
	response := openLocalStream(t, client, server, context.Background(), "/large", localPolicy(server))
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	var f *fault.Fault
	if string(body) != "1234" || !errors.As(err, &f) || f.Code != "platform.httpclient.stream_body_too_large" {
		t.Fatalf("expected 4 bytes and limit Fault, got %q, %v", body, err)
	}
	stats := response.Stats()
	if stats.BytesRead != 4 || stats.TerminationReason != "body_limit" {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestOpenStreamIdleReadTimeout(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		close(ready)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, 40*time.Millisecond, 2)
	response := openLocalStream(t, client, server, context.Background(), "/idle", localPolicy(server))
	defer response.Body.Close()
	<-ready
	var buffer [8]byte
	_, err := response.Body.Read(buffer[:])
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_idle_timeout" {
		t.Fatalf("expected idle timeout Fault, got %v", err)
	}
}

func TestOpenStreamRedirectCannotChangeOrigin(t *testing.T) {
	var privateReached atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/private" {
			privateReached.Store(true)
			return
		}
		http.Redirect(w, r, "http://127.0.0.1:"+strings.TrimPrefix(r.Host, "example.com:")+"/private", http.StatusFound)
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	policy := localPolicy(server)
	policy.AllowRedirects = true
	policy.MaxRedirects = 1
	_, err := client.OpenStream(context.Background(), localRequest(t, server, "/redirect"), policy)
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_destination_rejected" {
		t.Fatalf("expected redirect rejection, got %v", err)
	}
	if privateReached.Load() {
		t.Fatal("cross-origin target must not be requested")
	}
}

func TestOpenStreamPreservesRequestIDAndSafeStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	ctx := observability.WithRequestID(context.Background(), "stream-test-123")
	response := openLocalStream(t, client, server, ctx, "/safe?token=do-not-log", localPolicy(server))
	_, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	stats := response.Stats()
	if stats.RequestID != "stream-test-123" || stats.Dependency != "news.fetch" || stats.BytesRead != 2 || stats.TerminationReason != "eof" {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if strings.Contains(strings.Join([]string{stats.RequestID, stats.Dependency, stats.TerminationReason}, " "), "do-not-log") {
		t.Fatal("stats leaked URL query")
	}
}

func TestStreamOpenFaultRetryabilityRespectsIdempotency(t *testing.T) {
	err := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	for _, test := range []struct {
		idempotent bool
		retryable  bool
	}{
		{idempotent: false, retryable: false},
		{idempotent: true, retryable: true},
	} {
		policy := DestinationPolicy{Dependency: "news.fetch", Idempotent: test.idempotent}
		got := (&Client{}).streamOpenFault(err, context.Background(), policy)
		if got.Class != fault.ClassUnavailable || got.Retryable != test.retryable {
			t.Fatalf("streamOpenFault(idempotent=%v) = class %q retryable %v, want unavailable/%v",
				test.idempotent, got.Class, got.Retryable, test.retryable)
		}
		if err := got.Validate(); err != nil {
			t.Fatalf("streamOpenFault returned invalid Fault: %v", err)
		}
	}
}

func TestStreamOpenErrorClassificationCompatibility(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want streamOpenErrorKind
	}{
		{name: "response headers too large", err: errors.New("net/http: response headers exceeded 1024 bytes"), want: streamOpenErrorHeadersTooLarge},
		{name: "TLS handshake timeout", err: errors.New("net/http: TLS handshake timeout"), want: streamOpenErrorTLSHandshakeTimeout},
		{name: "response header timeout", err: errors.New("net/http: timeout awaiting response headers"), want: streamOpenErrorResponseHeaderTimeout},
		{name: "unrelated error", err: errors.New("connection reset"), want: streamOpenErrorOther},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyStreamOpenError(test.err); got != test.want {
				t.Fatalf("classification = %d, want %d", got, test.want)
			}
		})
	}
}

func TestOpenStreamIdleTimeoutExcludesCallerProcessingTime(t *testing.T) {
	firstWritten := make(chan struct{})
	allowSecond := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		close(firstWritten)
		<-allowSecond
		_, _ = w.Write([]byte("second"))
		w.(http.Flusher).Flush()
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, 50*time.Millisecond, 1)
	response := openLocalStream(t, client, server, context.Background(), "/caller-processing", localPolicy(server))
	defer response.Body.Close()
	<-firstWritten
	buffer := make([]byte, 5)
	if n, err := response.Body.Read(buffer); err != nil || string(buffer[:n]) != "first" {
		t.Fatalf("first read = %q, %v", buffer[:n], err)
	}
	// 调用方处理数据的时间不属于一次阻塞 Read 的空闲时间。
	time.Sleep(100 * time.Millisecond)
	close(allowSecond)
	buffer = make([]byte, 6)
	n, err := response.Body.Read(buffer)
	if string(buffer[:n]) != "second" || (err != nil && !errors.Is(err, io.EOF)) {
		t.Fatalf("second read after caller processing = %q, %v", buffer[:n], err)
	}
}

func TestOpenStreamHTTP2CloseInterruptsBlockedRead(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		close(ready)
		<-r.Context().Done()
	}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 1)
	serverTransport := server.Client().Transport.(*http.Transport)
	client.transport.TLSClientConfig = serverTransport.TLSClientConfig.Clone()
	client.transport.TLSClientConfig.ServerName = "127.0.0.1"
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	port := server.Listener.Addr().(*net.TCPAddr).Port
	request, err := http.NewRequest(http.MethodGet, "https://example.com:"+strconv.Itoa(port)+"/h2-close-blocked-read", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.OpenStream(context.Background(), request, localPolicy(server))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP/2 handler did not receive the request")
	}
	readResult := make(chan error, 1)
	go func() {
		var buffer [8]byte
		_, err := response.Body.Read(buffer[:])
		readResult <- err
	}()
	if err := response.Body.Close(); err != nil {
		t.Fatalf("HTTP/2 Close failed: %v", err)
	}
	select {
	case err := <-readResult:
		if err == nil {
			t.Fatal("HTTP/2 blocked Read unexpectedly succeeded after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("HTTP/2 Close did not interrupt blocked Read")
	}
}

func TestOpenStreamHeaderTimeoutIsClassified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	client.config.ResponseHeaderTimeout = 40 * time.Millisecond
	client.transport.ResponseHeaderTimeout = 40 * time.Millisecond
	_, err := client.OpenStream(context.Background(), localRequest(t, server, "/slow-header"), localPolicy(server))
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_header_timeout" {
		t.Fatalf("expected header timeout Fault, got %v", err)
	}
}

func TestOpenStreamCloseInterruptsBlockedRead(t *testing.T) {
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		close(ready)
		<-r.Context().Done()
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 1)
	response := openLocalStream(t, client, server, context.Background(), "/close-blocked-read", localPolicy(server))
	<-ready

	readResult := make(chan error, 1)
	go func() {
		var buffer [8]byte
		_, err := response.Body.Read(buffer[:])
		readResult <- err
	}()
	if err := response.Body.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	select {
	case err := <-readResult:
		if err == nil {
			t.Fatal("blocked Read unexpectedly succeeded after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not interrupt blocked Read")
	}
}

func TestOpenStreamConcurrentCloseIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.(http.Flusher).Flush(); <-r.Context().Done() }))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 4)
	responses := make([]*StreamResponse, 4)
	for i := range responses {
		responses[i] = openLocalStream(t, client, server, context.Background(), "/parallel", localPolicy(server))
	}
	var wait sync.WaitGroup
	for _, response := range responses {
		response := response
		wait.Add(2)
		go func() { defer wait.Done(); _ = response.Body.Close() }()
		go func() { defer wait.Done(); _ = response.Body.Close() }()
	}
	wait.Wait()
	for i := 0; i < 4; i++ {
		response := openLocalStream(t, client, server, context.Background(), "/reopen", localPolicy(server))
		_ = response.Body.Close()
	}
}

func TestOpenStreamRejectsInvalidConfiguration(t *testing.T) {
	if _, err := New(Config{MaxConcurrentStreams: maxConcurrentStreamsLimit + 1}); err == nil {
		t.Fatal("expected stream concurrency validation")
	}
	if _, err := New(Config{MaxStreamBytes: maxStreamBytesLimit + 1}); err == nil {
		t.Fatal("expected stream byte limit validation")
	}
	if _, err := New(Config{StreamTimeout: maxStreamTimeout + time.Second}); err == nil {
		t.Fatal("expected stream timeout validation")
	}
	if _, err := New(Config{StreamIdleTimeout: maxStreamIdleTimeout + time.Second}); err == nil {
		t.Fatal("expected idle timeout validation")
	}
}

func TestOpenStreamSlowConsumerDoesNotPrebuffer(t *testing.T) {
	firstWritten := make(chan struct{})
	allowFinish := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("first"))
		w.(http.Flusher).Flush()
		close(firstWritten)
		<-allowFinish
		_, _ = w.Write([]byte(strings.Repeat("x", 256<<10)))
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1<<20, time.Second, 2)
	response := openLocalStream(t, client, server, context.Background(), "/slow-consumer", localPolicy(server))
	defer response.Body.Close()
	<-firstWritten
	if got := response.Stats().BytesRead; got != 0 {
		t.Fatalf("body was consumed before caller Read: %d", got)
	}
	buffer := make([]byte, 5)
	n, err := response.Body.Read(buffer)
	if err != nil || string(buffer[:n]) != "first" {
		t.Fatalf("expected first chunk, got %q, %v", buffer[:n], err)
	}
	if got := response.Stats().BytesRead; got != 5 {
		t.Fatalf("expected caller-consumed byte count 5, got %d", got)
	}
	close(allowFinish)
	_, _ = io.Copy(io.Discard, response.Body)
}

func TestOpenStreamRejectsOversizedResponseHeaders(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Large", strings.Repeat("x", 4096))
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	client.config.MaxResponseHeaderBytes = 1024
	client.transport.MaxResponseHeaderBytes = 1024
	_, err := client.OpenStream(context.Background(), localRequest(t, server, "/large-headers"), localPolicy(server))
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.stream_headers_too_large" {
		t.Fatalf("expected response header size Fault, got %v", err)
	}
}

func TestOpenStreamBodyLimitCountsDecompressedBytes(t *testing.T) {
	var compressed bytes.Buffer
	writer := gzip.NewWriter(&compressed)
	if _, err := writer.Write([]byte("0123456789abcdef")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		_, _ = w.Write(compressed.Bytes())
	}))
	defer server.Close()

	client := newLocalStreamClient(t, server, 8, time.Second, 2)
	response := openLocalStream(t, client, server, context.Background(), "/gzip", localPolicy(server))
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	var platformFault *fault.Fault
	if len(body) != 8 || !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.stream_body_too_large" {
		t.Fatalf("expected decompressed byte limit, got %d bytes and %v", len(body), err)
	}
}

func TestOpenStreamDependencyWaitDoesNotBlockOtherDependencies(t *testing.T) {
	started := make(chan string, 3)
	releaseA := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- r.URL.Path
		w.(http.Flusher).Flush()
		if r.URL.Path == "/a1" {
			<-releaseA
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	client, err := New(Config{
		Resolver:      staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		StreamTimeout: 3 * time.Second, StreamIdleTimeout: time.Second,
		MaxStreamBytes: 1024, MaxConcurrentStreams: 2,
		StreamDependencyLimits: map[string]int{"dep.a": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	defer client.CloseIdleConnections()

	policyA := localPolicy(server)
	policyA.Dependency = "dep.a"
	first, err := client.OpenStream(context.Background(), localRequest(t, server, "/a1"), policyA)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Body.Close()
	select {
	case path := <-started:
		if path != "/a1" {
			t.Fatalf("expected /a1 to start, got %s", path)
		}
	case <-time.After(time.Second):
		t.Fatal("first dependency A request did not start")
	}

	secondRequest := localRequest(t, server, "/a2")
	secondCtx, secondCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer secondCancel()
	secondResult := make(chan error, 1)
	go func() {
		response, openErr := client.OpenStream(secondCtx, secondRequest, policyA)
		if openErr == nil {
			_ = response.Body.Close()
		}
		secondResult <- openErr
	}()
	// 等待第二个 A 请求进入额度等待；它不应占用全局槽位。
	time.Sleep(50 * time.Millisecond)

	policyB := localPolicy(server)
	policyB.Dependency = "dep.b"
	third, err := client.OpenStream(context.Background(), localRequest(t, server, "/b1"), policyB)
	if err != nil {
		t.Fatalf("dependency A waiter occupied a global slot: %v", err)
	}
	_ = third.Body.Close()
	select {
	case path := <-started:
		if path != "/b1" {
			t.Fatalf("expected dependency B to start, got %s", path)
		}
	case <-time.After(time.Second):
		t.Fatal("dependency B request did not reach server")
	}

	secondCancel()
	select {
	case err := <-secondResult:
		if err == nil {
			t.Fatal("expected waiting dependency A request to be canceled")
		}
	case <-time.After(time.Second):
		t.Fatal("dependency A waiter did not stop after cancellation")
	}
	close(releaseA)
}

func TestOpenStreamContextCancelReleasesSlotWithoutCallerClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	client := newLocalStreamClient(t, server, 1024, time.Second, 1)
	ctx, cancel := context.WithCancel(context.Background())
	first := openLocalStream(t, client, server, ctx, "/first", localPolicy(server))
	cancel()

	secondCtx, secondCancel := context.WithTimeout(context.Background(), time.Second)
	defer secondCancel()
	second, err := client.OpenStream(secondCtx, localRequest(t, server, "/second"), localPolicy(server))
	if err != nil {
		t.Fatalf("Context cancellation did not release the stream slot: %v", err)
	}
	_ = second.Body.Close()
	_ = first.Body.Close()
}

func TestBoundDoerOpenStreamUsesBoundPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("bound"))
	}))
	defer server.Close()

	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	bound, err := client.Bind(localPolicy(server))
	if err != nil {
		t.Fatal(err)
	}
	response, err := bound.OpenStream(context.Background(), localRequest(t, server, "/bound"))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "bound" {
		t.Fatalf("unexpected bound stream response %q, %v", body, err)
	}
}

func TestOpenStreamDoesNotRetryHTTPStatus(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("busy"))
	}))
	defer server.Close()

	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	policy := localPolicy(server)
	policy.Idempotent = true
	response := openLocalStream(t, client, server, context.Background(), "/no-retry", policy)
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable || requests.Load() != 1 {
		t.Fatalf("stream mode retried unexpectedly: status=%d requests=%d", response.StatusCode, requests.Load())
	}
}

func TestOpenStreamAllowsSameOriginRedirect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("final"))
	}))
	defer server.Close()

	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	policy := localPolicy(server)
	policy.AllowRedirects = true
	policy.MaxRedirects = 1
	response, err := client.OpenStream(context.Background(), localRequest(t, server, "/redirect"), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "final" {
		t.Fatalf("same-origin redirect failed: %q, %v", body, err)
	}
}

func TestOpenStreamFaultDoesNotExposeSensitiveQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("short"))
	}))
	defer server.Close()

	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	response := openLocalStream(t, client, server, context.Background(), "/truncated?api_key=secret-value", localPolicy(server))
	defer response.Body.Close()
	_, err := io.ReadAll(response.Body)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) {
		t.Fatalf("expected stream Fault, got %v", err)
	}
	if strings.Contains(platformFault.Error(), "secret-value") || strings.Contains(platformFault.SafeMessage, "secret-value") {
		t.Fatal("stream Fault exposed a sensitive URL query")
	}
}

func TestOpenStreamConnectTimeoutIsClassified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 1)
	client.config.DialTimeout = 40 * time.Millisecond
	client.dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	_, err := client.OpenStream(context.Background(), localRequest(t, server, "/connect-timeout"), localPolicy(server))
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.stream_connect_timeout" {
		t.Fatalf("expected connect timeout Fault, got %v", err)
	}
}

func TestOpenStreamTLSHandshakeTimeoutIsClassified(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	releaseServer := make(chan struct{})
	defer close(releaseServer)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer connection.Close()
		<-releaseServer
	}()

	port := listener.Addr().(*net.TCPAddr).Port
	client, err := New(Config{
		Resolver:            staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		TLSHandshakeTimeout: 40 * time.Millisecond,
		StreamTimeout:       time.Second,
		StreamIdleTimeout:   time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.CloseIdleConnections()
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, listener.Addr().String())
	}
	policy := DestinationPolicy{
		Dependency:   "news.fetch",
		AllowedHosts: []string{"example.com"},
		AllowedPorts: []uint16{uint16(port)},
	}
	request, err := http.NewRequest(http.MethodGet, "https://example.com:"+strconv.Itoa(port)+"/tls-timeout", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.OpenStream(context.Background(), request, policy)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.stream_tls_handshake_timeout" {
		t.Fatalf("expected TLS handshake timeout Fault, got %v", err)
	}
}

func TestOpenStreamStatsDurationIncludesResponseHeaderWait(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := newLocalStreamClient(t, server, 1024, time.Second, 1)
	response := openLocalStream(t, client, server, context.Background(), "/duration", localPolicy(server))
	defer response.Body.Close()
	if duration := response.Stats().Duration; duration < 60*time.Millisecond {
		t.Fatalf("stream duration omitted request/response-header wait: %s", duration)
	}
}
