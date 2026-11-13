package bootstrap

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/llm"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

type testPlatformLLMStreamOpener struct {
	response *httpclient.StreamResponse
	err      error
}

func (o testPlatformLLMStreamOpener) OpenStream(context.Context, *http.Request) (*httpclient.StreamResponse, error) {
	return o.response, o.err
}

func TestNewPlatformLLMStreamDoerRequiresBoundOpener(t *testing.T) {
	if _, err := newPlatformLLMStreamDoer(nil); err == nil {
		t.Fatal("nil Platform stream opener unexpectedly succeeded")
	}
}

func TestPlatformLLMStreamDoerTransfersResponseAndBodyOwnership(t *testing.T) {
	body := io.NopCloser(strings.NewReader("event: response.completed\ndata: {}\n\n"))
	header := http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-ID": []string{"gateway-req"}}
	opener := testPlatformLLMStreamOpener{
		response: &httpclient.StreamResponse{StatusCode: http.StatusOK, Header: header, Body: body},
	}
	doer, err := newPlatformLLMStreamDoer(opener)
	if err != nil {
		t.Fatalf("newPlatformLLMStreamDoer() error = %v", err)
	}
	response, err := doer.OpenStream(context.Background(), &http.Request{})
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	if response == nil || response.StatusCode != http.StatusOK || response.Body != body ||
		response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected LLM stream response: %+v", response)
	}
	header.Set("Content-Type", "application/json")
	if response.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("LLM response header aliases mutable Platform header")
	}
	if _, err := io.ReadAll(response.Body); err != nil {
		t.Fatalf("read transferred response body: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close transferred response body: %v", err)
	}
}

func TestPlatformLLMStreamDoerPreservesResponseOnErrorForCleanup(t *testing.T) {
	upstreamErr := errors.New("upstream stream failed")
	body := io.NopCloser(strings.NewReader("partial"))
	doer, err := newPlatformLLMStreamDoer(testPlatformLLMStreamOpener{
		response: &httpclient.StreamResponse{StatusCode: http.StatusBadGateway, Header: http.Header{}, Body: body},
		err:      upstreamErr,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, gotErr := doer.OpenStream(context.Background(), &http.Request{})
	if !errors.Is(gotErr, upstreamErr) || response == nil || response.Body != body {
		t.Fatalf("OpenStream() = (%+v, %v), want partial response and original error", response, gotErr)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close partial response body: %v", err)
	}
}

var _ llm.HTTPStreamDoer = platformLLMStreamDoer{}
