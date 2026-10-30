package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

func newProviderProbeHTTPTestHandler() http.Handler {
	svc := service.NewWithCapabilities(
		nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000,
		service.ResilienceConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	server := NewWithCapabilityInterfaces(
		"probe-test-key", "forward_adjusted", svc,
		nil, nil, nil, 2000, 110*time.Second,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	return server.Handler()
}

func performProviderProbeRequest(handler http.Handler, body string, withAPIKey bool) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/data-sources/probe", bytes.NewBufferString(body))
	if withAPIKey {
		req.Header.Set("X-API-Key", "probe-test-key")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}

func TestProviderProbeEndpointRequiresAuthentication(t *testing.T) {
	response := performProviderProbeRequest(newProviderProbeHTTPTestHandler(), "", false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("missing API key status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestProviderProbeEndpointKeepsEmptyBodyCompatibility(t *testing.T) {
	response := performProviderProbeRequest(newProviderProbeHTTPTestHandler(), "", true)
	if response.Code != http.StatusOK {
		t.Fatalf("empty request body should retain default behavior, status=%d body=%s", response.Code, response.Body.String())
	}
	for _, expected := range []string{`"scope":"all"`, `"timeout_seconds":12`, `"requested_capabilities":`, `"total":22`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("default probe response missing %s: %s", expected, response.Body.String())
		}
	}
}

func TestProviderProbeEndpointFiltersCoreScope(t *testing.T) {
	response := performProviderProbeRequest(newProviderProbeHTTPTestHandler(), `{"scope":"core"}`, true)
	if response.Code != http.StatusOK {
		t.Fatalf("core scope status=%d body=%s", response.Code, response.Body.String())
	}
	for _, expected := range []string{`"scope":"core"`, `"total":6`, `"capability":"quote"`, `"capability":"daily_bars"`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("core probe response missing %s: %s", expected, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), `"capability":"stock_universe"`) {
		t.Fatalf("core scope must not include full market universe: %s", response.Body.String())
	}
}

func TestProviderProbeEndpointUsesCapabilityAllowList(t *testing.T) {
	response := performProviderProbeRequest(newProviderProbeHTTPTestHandler(), `{"scope":"all","capabilities":["quote"],"timeout_seconds":5}`, true)
	if response.Code != http.StatusOK {
		t.Fatalf("capability filter status=%d body=%s", response.Code, response.Body.String())
	}
	for _, expected := range []string{`"scope":"all"`, `"timeout_seconds":5`, `"requested_capabilities":["quote"]`, `"total":1`, `"capability":"quote"`} {
		if !strings.Contains(response.Body.String(), expected) {
			t.Fatalf("filtered probe response missing %s: %s", expected, response.Body.String())
		}
	}
	if strings.Contains(response.Body.String(), `"capability":"daily_bars"`) {
		t.Fatalf("capability allow-list must exclude unrequested capabilities: %s", response.Body.String())
	}
}

func TestProviderProbeEndpointRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"scope":`},
		{name: "non-object JSON", body: `null`},
		{name: "unknown field", body: `{"scope":"all","unknown":true}`},
		{name: "invalid scope", body: `{"scope":"everything"}`},
		{name: "unknown capability", body: `{"capabilities":["unknown_capability"]}`},
		{name: "duplicate capability", body: `{"capabilities":["quote","quote"]}`},
		{name: "capability outside core", body: `{"scope":"core","capabilities":["stock_universe"]}`},
		{name: "timeout too small", body: `{"timeout_seconds":0.5}`},
		{name: "timeout too large", body: `{"timeout_seconds":31}`},
		{name: "multiple JSON values", body: `{"scope":"core"} {}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := performProviderProbeRequest(newProviderProbeHTTPTestHandler(), test.body, true)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), `"code":"VALIDATION_ERROR"`) {
				t.Fatalf("invalid request status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestProviderProbeEndpointRejectsOversizedBody(t *testing.T) {
	body := `{"scope":"all","capabilities":["quote"],"padding":"` + strings.Repeat("x", 9*1024) + `"}`
	response := performProviderProbeRequest(newProviderProbeHTTPTestHandler(), body, true)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("oversized body status=%d body=%s", response.Code, response.Body.String())
	}
}
