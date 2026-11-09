package httpclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
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
		Resolver: staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		StreamTimeout: 3 * time.Second,
		StreamIdleTimeout: idle,
		MaxStreamBytes: maxBytes,
		MaxConcurrentStreams: maxStreams,
	})
	if err != nil { t.Fatal(err) }
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func openLocalStream(t *testing.T, client *Client, server *httptest.Server, ctx context.Context, path string, policy DestinationPolicy) *StreamResponse {
	t.Helper()
	response, err := client.OpenStream(ctx, localRequest(t, server, path), policy)
	if err != nil { t.Fatal(err) }
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
	if err != nil || string(buffer[:n]) != "first" { t.Fatalf("expected first chunk before EOF, got %q, %v", buffer[:n], err) }
	select { case <-allowFinish: t.Fatal("server should still be waiting"); default: }
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
		if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_canceled" { t.Fatalf("expected canceled Fault, got %v", err) }
	case <-time.After(time.Second): t.Fatal("blocked Read did not stop after cancellation")
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
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_deadline_exceeded" { t.Fatalf("expected deadline Fault, got %v", err) }
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
		if err != nil { secondError <- err; return }
		secondResult <- response
	}()
	select {
	case <-secondResult: t.Fatal("second stream bypassed occupied slot")
	case err := <-secondError: t.Fatalf("second stream failed: %v", err)
	case <-time.After(50*time.Millisecond):
	}
	if err := first.Body.Close(); err != nil { t.Fatal(err) }
	select {
	case second := <-secondResult: _ = second.Body.Close()
	case err := <-secondError: t.Fatal(err)
	case <-time.After(time.Second): t.Fatal("slot was not released after Close")
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
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_upstream_truncated" { t.Fatalf("expected truncation Fault, got %v", err) }
}

func TestOpenStreamStopsAtCumulativeBodyLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("12345")) }))
	defer server.Close()
	client := newLocalStreamClient(t, server, 4, time.Second, 2)
	response := openLocalStream(t, client, server, context.Background(), "/large", localPolicy(server))
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	var f *fault.Fault
	if string(body) != "1234" || !errors.As(err, &f) || f.Code != "platform.httpclient.stream_body_too_large" { t.Fatalf("expected 4 bytes and limit Fault, got %q, %v", body, err) }
	stats := response.Stats()
	if stats.BytesRead != 4 || stats.TerminationReason != "body_limit" { t.Fatalf("unexpected stats: %+v", stats) }
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
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_idle_timeout" { t.Fatalf("expected idle timeout Fault, got %v", err) }
}

func TestOpenStreamRedirectCannotChangeOrigin(t *testing.T) {
	var privateReached atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/private" { privateReached.Store(true); return }
		http.Redirect(w, r, "http://127.0.0.1:"+strings.TrimPrefix(r.Host, "example.com:")+"/private", http.StatusFound)
	}))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	policy := localPolicy(server)
	policy.AllowRedirects = true
	policy.MaxRedirects = 1
	_, err := client.OpenStream(context.Background(), localRequest(t, server, "/redirect"), policy)
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_destination_rejected" { t.Fatalf("expected redirect rejection, got %v", err) }
	if privateReached.Load() { t.Fatal("cross-origin target must not be requested") }
}

func TestOpenStreamPreservesRequestIDAndSafeStats(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	ctx := observability.WithRequestID(context.Background(), "stream-test-123")
	response := openLocalStream(t, client, server, ctx, "/safe?token=do-not-log", localPolicy(server))
	_, err := io.ReadAll(response.Body)
	if err != nil { t.Fatal(err) }
	stats := response.Stats()
	if stats.RequestID != "stream-test-123" || stats.Dependency != "news.fetch" || stats.BytesRead != 2 || stats.TerminationReason != "eof" { t.Fatalf("unexpected stats: %+v", stats) }
	if strings.Contains(strings.Join([]string{stats.RequestID, stats.Dependency, stats.TerminationReason}, " "), "do-not-log") { t.Fatal("stats leaked URL query") }
}

func TestOpenStreamHeaderTimeoutIsClassified(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 2)
	client.config.ResponseHeaderTimeout = 40*time.Millisecond
	client.transport.ResponseHeaderTimeout = 40*time.Millisecond
	_, err := client.OpenStream(context.Background(), localRequest(t, server, "/slow-header"), localPolicy(server))
	var f *fault.Fault
	if !errors.As(err, &f) || f.Code != "platform.httpclient.stream_header_timeout" { t.Fatalf("expected header timeout Fault, got %v", err) }
}

func TestOpenStreamConcurrentCloseIsIdempotent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.(http.Flusher).Flush(); <-r.Context().Done() }))
	defer server.Close()
	client := newLocalStreamClient(t, server, 1024, time.Second, 4)
	responses := make([]*StreamResponse, 4)
	for i := range responses { responses[i] = openLocalStream(t, client, server, context.Background(), "/parallel", localPolicy(server)) }
	var wait sync.WaitGroup
	for _, response := range responses {
		response := response
		wait.Add(2)
		go func(){ defer wait.Done(); _ = response.Body.Close() }()
		go func(){ defer wait.Done(); _ = response.Body.Close() }()
	}
	wait.Wait()
	for i:=0;i<4;i++ { response:=openLocalStream(t,client,server,context.Background(),"/reopen",localPolicy(server)); _=response.Body.Close() }
}

func TestOpenStreamRejectsInvalidConfiguration(t *testing.T) {
	if _,err:=New(Config{MaxConcurrentStreams:maxConcurrentStreamsLimit+1});err==nil { t.Fatal("expected stream concurrency validation") }
	if _,err:=New(Config{MaxStreamBytes:maxStreamBytesLimit+1});err==nil { t.Fatal("expected stream byte limit validation") }
	if _,err:=New(Config{StreamTimeout:maxStreamTimeout+time.Second});err==nil { t.Fatal("expected stream timeout validation") }
	if _,err:=New(Config{StreamIdleTimeout:maxStreamIdleTimeout+time.Second});err==nil { t.Fatal("expected idle timeout validation") }
}

func TestOpenStreamSlowConsumerDoesNotPrebuffer(t *testing.T) {
	firstWritten:=make(chan struct{})
	allowFinish:=make(chan struct{})
	server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,_ *http.Request){
		_,_=w.Write([]byte("first"));w.(http.Flusher).Flush();close(firstWritten)
		<-allowFinish
		_,_=w.Write([]byte(strings.Repeat("x",256<<10)))
	}))
	defer server.Close()
	client:=newLocalStreamClient(t,server,1<<20,time.Second,2)
	response:=openLocalStream(t,client,server,context.Background(),"/slow-consumer",localPolicy(server))
	defer response.Body.Close()
	<-firstWritten
	if got:=response.Stats().BytesRead;got!=0 { t.Fatalf("body was consumed before caller Read: %d",got) }
	buffer:=make([]byte,5)
	n,err:=response.Body.Read(buffer)
	if err!=nil||string(buffer[:n])!="first" { t.Fatalf("expected first chunk, got %q, %v",buffer[:n],err) }
	if got:=response.Stats().BytesRead;got!=5 { t.Fatalf("expected caller-consumed byte count 5, got %d",got) }
	close(allowFinish)
	_,_=io.Copy(io.Discard,response.Body)
}
