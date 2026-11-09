package httpclient

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/observability"
)

const (
	defaultTimeout                = 15 * time.Second
	defaultDialTimeout            = 5 * time.Second
	defaultResponseHeaderTimeout  = 8 * time.Second
	defaultMaxResponseHeaderBytes = int64(1 << 20)
	maxResponseHeaderBytesLimit   = int64(4 << 20)
	defaultMaxResponseBytes       = int64(2 << 20)
	maxResponseBytesLimit         = int64(16 << 20)
	defaultMaxStreamBytes         = int64(16 << 20)
	maxStreamBytesLimit           = int64(64 << 20)
	defaultStreamTimeout          = 10 * time.Minute
	maxStreamTimeout              = 30 * time.Minute
	defaultStreamIdleTimeout      = 30 * time.Second
	maxStreamIdleTimeout          = 5 * time.Minute
	defaultMaxConcurrentStreams   = 2
	maxConcurrentStreamsLimit     = 8
	defaultMaxConcurrent          = 4
	maxConcurrentLimit            = 32
	maxDependencyLimitEntries     = 64
	maxRetryLimit                 = 2
)

var (
	errPolicyRejected   = errors.New("destination policy rejected")
	errDNSResolution    = errors.New("destination DNS resolution failed")
	errRedirectRejected = errors.New("redirect rejected")
	errResponseTooLarge = errors.New("response body exceeds configured limit")
)

type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type Config struct {
	Timeout                time.Duration
	DialTimeout            time.Duration
	TLSHandshakeTimeout    time.Duration
	ResponseHeaderTimeout  time.Duration
	MaxResponseHeaderBytes int64 // 共享 Transport 的响应头字节上限。
	MaxResponseBytes       int64
	MaxRetries             int
	MaxConcurrent          int
	DependencyLimits       map[string]int

	StreamTimeout          time.Duration  // 单条流的总生命周期上限，父 Context 可以进一步缩短。
	StreamIdleTimeout      time.Duration  // 单次阻塞 Read 等待上游数据的最长时间。
	MaxStreamBytes         int64          // 单条流允许交付的解压后累计字节上限。
	MaxConcurrentStreams   int            // 全局并发流上限，与缓冲请求并发池隔离。
	StreamDependencyLimits map[string]int // 按 Dependency 设置的并发流子上限。

	Resolver Resolver
}

// DestinationPolicy must be supplied for every request. Hosts are exact-match
// allowlist entries; wildcard hosts are intentionally unsupported.
type DestinationPolicy struct {
	Dependency     string
	AllowedHosts   []string
	AllowedPorts   []uint16
	AllowHTTP      bool
	AllowRedirects bool
	MaxRedirects   int
	Idempotent     bool
}

type Client struct {
	config                 Config
	resolver               Resolver
	dial                   func(context.Context, string, string) (net.Conn, error)
	httpClient             *http.Client
	transport              *http.Transport
	defaultSemaphore       chan struct{}
	semaphores             map[string]chan struct{}
	defaultStreamSemaphore chan struct{}
	streamSemaphores       map[string]chan struct{}
}

type policyContextKey struct{}

func New(config Config) (*Client, error) {
	if config.Timeout <= 0 {
		config.Timeout = defaultTimeout
	}
	if config.DialTimeout <= 0 {
		config.DialTimeout = defaultDialTimeout
	}
	if config.TLSHandshakeTimeout <= 0 {
		config.TLSHandshakeTimeout = 5 * time.Second
	}
	if config.ResponseHeaderTimeout <= 0 {
		config.ResponseHeaderTimeout = defaultResponseHeaderTimeout
	}
	if config.MaxResponseHeaderBytes <= 0 {
		config.MaxResponseHeaderBytes = defaultMaxResponseHeaderBytes
	}
	if config.MaxResponseHeaderBytes > maxResponseHeaderBytesLimit {
		return nil, errors.New("max response header bytes exceeds 4 MiB")
	}
	if config.MaxResponseBytes <= 0 {
		config.MaxResponseBytes = defaultMaxResponseBytes
	}
	if config.MaxResponseBytes > maxResponseBytesLimit {
		return nil, errors.New("max response bytes exceeds 16 MiB")
	}
	if config.MaxStreamBytes <= 0 {
		config.MaxStreamBytes = defaultMaxStreamBytes
	}
	if config.MaxStreamBytes > maxStreamBytesLimit {
		return nil, errors.New("max stream bytes exceeds 64 MiB")
	}
	if config.StreamTimeout <= 0 {
		config.StreamTimeout = defaultStreamTimeout
	}
	if config.StreamTimeout > maxStreamTimeout {
		return nil, errors.New("stream timeout exceeds 30 minutes")
	}
	if config.StreamIdleTimeout <= 0 {
		config.StreamIdleTimeout = defaultStreamIdleTimeout
	}
	if config.StreamIdleTimeout > maxStreamIdleTimeout {
		return nil, errors.New("stream idle timeout exceeds 5 minutes")
	}
	if config.MaxConcurrentStreams <= 0 {
		config.MaxConcurrentStreams = defaultMaxConcurrentStreams
	}
	if config.MaxConcurrentStreams > maxConcurrentStreamsLimit {
		return nil, fmt.Errorf("max concurrent streams cannot exceed %d", maxConcurrentStreamsLimit)
	}
	if config.MaxRetries < 0 || config.MaxRetries > maxRetryLimit {
		return nil, errors.New("max retries must be between 0 and 2")
	}
	if config.MaxConcurrent <= 0 {
		config.MaxConcurrent = defaultMaxConcurrent
	}
	if config.MaxConcurrent > maxConcurrentLimit {
		return nil, fmt.Errorf("max concurrent requests cannot exceed %d", maxConcurrentLimit)
	}

	if len(config.DependencyLimits) > maxDependencyLimitEntries {
		return nil, fmt.Errorf("dependency limits cannot exceed %d entries", maxDependencyLimitEntries)
	}

	limits := make(map[string]int, len(config.DependencyLimits))
	for dependency, limit := range config.DependencyLimits {
		if !validDependency(dependency) || limit < 1 || limit > maxConcurrentLimit {
			return nil, errors.New("invalid dependency concurrency limit")
		}
		limits[dependency] = limit
	}
	config.DependencyLimits = limits

	if len(config.StreamDependencyLimits) > maxDependencyLimitEntries {
		return nil, fmt.Errorf("stream dependency limits cannot exceed %d entries", maxDependencyLimitEntries)
	}
	streamLimits := make(map[string]int, len(config.StreamDependencyLimits))
	for dependency, limit := range config.StreamDependencyLimits {
		if !validDependency(dependency) || limit < 1 || limit > config.MaxConcurrentStreams {
			return nil, errors.New("invalid stream dependency concurrency limit")
		}
		streamLimits[dependency] = limit
	}
	config.StreamDependencyLimits = streamLimits

	resolver := config.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	dialer := &net.Dialer{Timeout: config.DialTimeout, KeepAlive: 30 * time.Second}
	semaphores := make(map[string]chan struct{}, len(limits))
	for dependency, limit := range limits {
		semaphores[dependency] = make(chan struct{}, limit)
	}
	streamSemaphores := make(map[string]chan struct{}, len(streamLimits))
	for dependency, limit := range streamLimits {
		streamSemaphores[dependency] = make(chan struct{}, limit)
	}
	client := &Client{
		config:                 config,
		resolver:               resolver,
		dial:                   dialer.DialContext,
		defaultSemaphore:       make(chan struct{}, config.MaxConcurrent),
		semaphores:             semaphores,
		defaultStreamSemaphore: make(chan struct{}, config.MaxConcurrentStreams),
		streamSemaphores:       streamSemaphores,
	}

	transport := &http.Transport{
		Proxy:                  nil, // 不读取 HTTP_PROXY/HTTPS_PROXY，避免不可信代理绕过目的地址校验。
		DialContext:            client.dialContext,
		TLSClientConfig:        &tls.Config{MinVersion: tls.VersionTLS12},
		TLSHandshakeTimeout:    config.TLSHandshakeTimeout,
		ResponseHeaderTimeout:  config.ResponseHeaderTimeout,
		MaxResponseHeaderBytes: config.MaxResponseHeaderBytes,
		IdleConnTimeout:        90 * time.Second,
		MaxIdleConns:           64,
		MaxIdleConnsPerHost:    4,
		MaxConnsPerHost:        16,
		ForceAttemptHTTP2:      true,
	}
	client.transport = transport
	client.httpClient = &http.Client{
		Transport: transport,
		CheckRedirect: func(request *http.Request, via []*http.Request) error {
			return validateRedirect(request, via)
		},
	}
	return client, nil
}

// Do executes one request after validating its destination and bounds its
// response body in memory. The caller must close the returned response body.
func (c *Client) Do(ctx context.Context, request *http.Request, policy DestinationPolicy) (*http.Response, error) {
	if ctx == nil || request == nil {
		return nil, fault.New(fault.ClassValidation, "platform.httpclient.invalid_request", "invalid HTTP request", nil)
	}
	if request.URL == nil {
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
	if (request.Host != "" && !strings.EqualFold(request.Host, request.URL.Host)) || request.Header.Get("Host") != "" {
		return nil, c.newFault(fault.ClassValidation, "platform.httpclient.custom_host_rejected", "custom Host header is not allowed", errPolicyRejected, policy.Dependency, false)
	}
	if hasHeaderFold(request.Header, "Accept-Encoding") {
		return nil, c.newFault(fault.ClassValidation, "platform.httpclient.accept_encoding_rejected", "custom Accept-Encoding header is not allowed", errPolicyRejected, policy.Dependency, false)
	}
	if err := validateURL(request.URL, policy); err != nil {
		return nil, c.faultFor(err, policy.Dependency)
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.config.Timeout)
	defer cancel()
	requestCtx = context.WithValue(requestCtx, policyContextKey{}, policy)

	semaphore := c.semaphore(policy.Dependency)
	held := false
	acquire := func() error {
		select {
		case semaphore <- struct{}{}:
			held = true
			return nil
		case <-requestCtx.Done():
			return requestCtx.Err()
		}
	}
	release := func() {
		if held {
			<-semaphore
			held = false
		}
	}
	if err := acquire(); err != nil {
		return nil, c.faultFor(err, policy.Dependency)
	}
	defer release()

	retryEnabled := policy.Idempotent && c.config.MaxRetries > 0 && (request.Body == nil || request.GetBody != nil)
	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		current := request.Clone(requestCtx)
		observability.PropagateRequestID(requestCtx, current)
		if attempt > 0 && request.Body != nil {
			body, err := request.GetBody()
			if err != nil {
				return nil, c.faultFor(err, policy.Dependency)
			}
			current.Body = body
		}

		response, err := c.httpClient.Do(current)
		if err != nil {
			if retryEnabled && attempt < c.config.MaxRetries && retryableTransportError(err) {
				release()
				if waitErr := waitRetry(requestCtx, attempt); waitErr != nil {
					return nil, c.faultFor(waitErr, policy.Dependency)
				}
				if acquireErr := acquire(); acquireErr != nil {
					return nil, c.faultFor(acquireErr, policy.Dependency)
				}
				continue
			}
			return nil, c.faultFor(err, policy.Dependency)
		}

		if retryEnabled && attempt < c.config.MaxRetries && retryableStatus(response.StatusCode) {
			_ = response.Body.Close()
			release()
			if waitErr := waitRetry(requestCtx, attempt); waitErr != nil {
				return nil, c.faultFor(waitErr, policy.Dependency)
			}
			if acquireErr := acquire(); acquireErr != nil {
				return nil, c.faultFor(acquireErr, policy.Dependency)
			}
			continue
		}

		body, readErr := readLimitedBody(response.Body, c.config.MaxResponseBytes)
		_ = response.Body.Close()
		if readErr != nil {
			return nil, c.faultFor(readErr, policy.Dependency)
		}
		response.Body = io.NopCloser(bytes.NewReader(body))
		response.ContentLength = int64(len(body))
		return response, nil
	}
	return nil, c.faultFor(context.DeadlineExceeded, policy.Dependency)
}

func (c *Client) CloseIdleConnections() {
	if c != nil && c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

// semaphore returns a dedicated slot pool only for explicitly configured
// dependencies. Unconfigured dependency names share one bounded fallback pool,
// so request-controlled identifiers cannot grow the semaphore map.
func (c *Client) semaphore(dependency string) chan struct{} {
	if semaphore, ok := c.semaphores[dependency]; ok {
		return semaphore
	}
	return c.defaultSemaphore
}

func (c *Client) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, c.config.DialTimeout)
	defer cancel()

	policy, ok := dialCtx.Value(policyContextKey{}).(DestinationPolicy)
	if !ok {
		return nil, errPolicyRejected
	}
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, errPolicyRejected
	}
	addresses, err := c.resolveAndValidate(dialCtx, host, port, policy)
	if err != nil {
		if dialCtx.Err() != nil {
			return nil, dialCtx.Err()
		}
		return nil, err
	}

	var lastErr error
	for _, ip := range addresses {
		connection, dialErr := c.dial(dialCtx, network, net.JoinHostPort(ip.String(), port))
		if dialErr == nil {
			return connection, nil
		}
		lastErr = dialErr
		if dialCtx.Err() != nil {
			return nil, dialCtx.Err()
		}
	}
	if lastErr == nil {
		return nil, errDNSResolution
	}
	return nil, lastErr
}

func (c *Client) resolveAndValidate(ctx context.Context, host, port string, policy DestinationPolicy) ([]netip.Addr, error) {
	host = normalizeHost(host)
	if !validHost(host) || !hostAllowed(host, policy.AllowedHosts) || !portAllowed(port, policy) {
		return nil, errPolicyRejected
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !isPublicIP(ip) {
			return nil, errPolicyRejected
		}
		return []netip.Addr{ip.Unmap()}, nil
	}

	addresses, err := c.resolver.LookupNetIP(ctx, "ip", host)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil || len(addresses) == 0 {
		return nil, errDNSResolution
	}
	normalized := make([]netip.Addr, 0, len(addresses))
	for _, address := range addresses {
		if !isPublicIP(address) {
			return nil, errPolicyRejected
		}
		normalized = append(normalized, address.Unmap())
	}
	return normalized, nil
}

func (c *Client) faultFor(err error, dependency string) *fault.Fault {
	switch {
	case errors.Is(err, errPolicyRejected), errors.Is(err, errRedirectRejected):
		return c.newFault(fault.ClassValidation, "platform.httpclient.destination_rejected", "destination is not allowed", err, dependency, false)
	case errors.Is(err, errDNSResolution):
		return c.newFault(fault.ClassUnavailable, "platform.httpclient.dns_failed", "destination DNS resolution failed", err, dependency, false)
	case errors.Is(err, errResponseTooLarge):
		return c.newFault(fault.ClassValidation, "platform.httpclient.response_too_large", "response body exceeds configured limit", err, dependency, false)
	case errors.Is(err, context.DeadlineExceeded):
		return c.newFault(fault.ClassTimeout, "platform.httpclient.timeout", "HTTP request timed out", err, dependency, false)
	case errors.Is(err, context.Canceled):
		return c.newFault(fault.ClassUnavailable, "platform.httpclient.canceled", "HTTP request was canceled", err, dependency, false)
	default:
		return c.newFault(fault.ClassUnavailable, "platform.httpclient.request_failed", "HTTP dependency request failed", err, dependency, retryableTransportError(err))
	}
}

func (c *Client) newFault(class fault.Class, code, message string, cause error, dependency string, retryable bool) *fault.Fault {
	return &fault.Fault{
		Class: class, Code: code, SafeMessage: message,
		Dependency: dependency, Retryable: retryable, Cause: cause,
	}
}

func hasHeaderFold(header http.Header, name string) bool {
	for key := range header {
		if strings.EqualFold(key, name) {
			return true
		}
	}
	return false
}

func validDependency(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func retryableTransportError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, errPolicyRejected) || errors.Is(err, errDNSResolution) ||
		errors.Is(err, errRedirectRejected) {
		return false
	}
	var networkError net.Error
	return errors.As(err, &networkError) || errors.Is(err, io.EOF)
}

func retryableStatus(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable ||
		status == http.StatusGatewayTimeout
}

func waitRetry(ctx context.Context, attempt int) error {
	delay := time.Duration(attempt+1) * 100 * time.Millisecond
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func readLimitedBody(body io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, errResponseTooLarge
	}
	return data, nil
}

func validateRedirect(request *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return errRedirectRejected
	}
	policy, ok := request.Context().Value(policyContextKey{}).(DestinationPolicy)
	if !ok || !policy.AllowRedirects {
		return errRedirectRejected
	}
	maxRedirects := policy.MaxRedirects
	if maxRedirects <= 0 {
		maxRedirects = 3
	}
	if maxRedirects > 3 || len(via) > maxRedirects {
		return errRedirectRejected
	}
	previous := via[len(via)-1]
	if previous.URL.Scheme == "https" && request.URL.Scheme != "https" {
		return errRedirectRejected
	}
	// Redirects may carry Authorization, Cookie, or provider-specific headers.
	// Keep them within the original origin until cross-origin credential
	// forwarding has an explicit, separately reviewed policy.
	if !sameOrigin(previous.URL, request.URL) {
		return errRedirectRejected
	}
	if err := validateURL(request.URL, policy); err != nil {
		return errRedirectRejected
	}
	for _, prior := range via {
		if prior.URL.String() == request.URL.String() {
			return errRedirectRejected
		}
	}
	return nil
}

func sameOrigin(left, right *url.URL) bool {
	if left == nil || right == nil || !strings.EqualFold(left.Scheme, right.Scheme) {
		return false
	}
	leftPort := left.Port()
	if leftPort == "" {
		if strings.EqualFold(left.Scheme, "https") {
			leftPort = "443"
		} else if strings.EqualFold(left.Scheme, "http") {
			leftPort = "80"
		}
	}
	rightPort := right.Port()
	if rightPort == "" {
		if strings.EqualFold(right.Scheme, "https") {
			rightPort = "443"
		} else if strings.EqualFold(right.Scheme, "http") {
			rightPort = "80"
		}
	}
	return normalizeHost(left.Hostname()) == normalizeHost(right.Hostname()) && leftPort == rightPort
}

func validatePolicy(policy DestinationPolicy) error {
	if len(policy.AllowedHosts) == 0 || len(policy.AllowedHosts) > 64 || len(policy.AllowedPorts) > 16 {
		return errPolicyRejected
	}
	for _, host := range policy.AllowedHosts {
		normalized := normalizeHost(host)
		if !validHost(normalized) || strings.Contains(normalized, "*") {
			return errPolicyRejected
		}
	}
	for _, port := range policy.AllowedPorts {
		if port == 0 {
			return errPolicyRejected
		}
	}
	if policy.MaxRedirects < 0 || policy.MaxRedirects > 3 {
		return errPolicyRejected
	}
	if !policy.AllowRedirects && policy.MaxRedirects > 0 {
		return errPolicyRejected
	}
	return nil
}

func validateURL(value *url.URL, policy DestinationPolicy) error {
	if value == nil || value.Opaque != "" || value.User != nil || value.Hostname() == "" {
		return errPolicyRejected
	}
	if value.Scheme != "https" && !(value.Scheme == "http" && policy.AllowHTTP) {
		return errPolicyRejected
	}
	host := normalizeHost(value.Hostname())
	if !validHost(host) || !hostAllowed(host, policy.AllowedHosts) {
		return errPolicyRejected
	}
	port := value.Port()
	if port == "" {
		if value.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	if !portAllowed(port, policy) {
		return errPolicyRejected
	}
	if ip, err := netip.ParseAddr(host); err == nil && !isPublicIP(ip) {
		return errPolicyRejected
	}
	return nil
}

func portAllowed(port string, policy DestinationPolicy) bool {
	value, err := strconv.Atoi(port)
	if err != nil || value < 1 || value > 65535 {
		return false
	}
	if len(policy.AllowedPorts) > 0 {
		for _, allowed := range policy.AllowedPorts {
			if int(allowed) == value {
				return true
			}
		}
		return false
	}
	if value == 443 {
		return true
	}
	return value == 80 && policy.AllowHTTP
}

func hostAllowed(host string, allowedHosts []string) bool {
	for _, allowed := range allowedHosts {
		if normalizeHost(allowed) == host {
			return true
		}
	}
	return false
}

func normalizeHost(host string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
}

func validHost(host string) bool {
	if host == "" || len(host) > 253 || strings.ContainsAny(host, "%/\\") {
		return false
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if !((char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-') {
				return false
			}
		}
	}
	return true
}
