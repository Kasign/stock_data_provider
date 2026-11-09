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
		Resolver: staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
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
