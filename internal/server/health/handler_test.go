package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	serverdiagnostics "example.com/stock-analysis-flow/internal/server/diagnostics"
)

type fakeReadinessChecker struct {
	report serverdiagnostics.Report
	err    error
	calls  int
}

func (f *fakeReadinessChecker) Run(context.Context) (serverdiagnostics.Report, error) {
	f.calls++
	return f.report, f.err
}

func TestHealthzDoesNotDependOnStorageReadiness(t *testing.T) {
	checker := &fakeReadinessChecker{err: errors.New("dependency unavailable")}
	handler, err := NewHandler(checker)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if checker.calls != 0 {
		t.Fatalf("readiness checker calls = %d, want 0", checker.calls)
	}
}

func TestDiagnosticsReturnsStructuredReportWhenChecksComplete(t *testing.T) {
	checker := &fakeReadinessChecker{report: serverdiagnostics.Report{
		Readiness: serverdiagnostics.Readiness{Ready: false, ReasonCode: "server.readiness.required_check_unhealthy", SafeMessage: "dependency unavailable"},
	}}
	handler, err := NewHandler(checker)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/diagnostics", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !contains(response.Body.String(), "readiness") || !contains(response.Body.String(), "dependency unavailable") {
		t.Fatalf("structured diagnostics report missing: %s", response.Body.String())
	}
}

func TestDiagnosticsHidesRunnerErrors(t *testing.T) {
	checker := &fakeReadinessChecker{err: errors.New("sensitive database connection details")}
	handler, err := NewHandler(checker)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/diagnostics", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if contains(response.Body.String(), "sensitive") {
		t.Fatalf("diagnostics response leaked internal error: %s", response.Body.String())
	}
}

func TestReadyzReturnsServiceUnavailableWhenNotReady(t *testing.T) {
	checker := &fakeReadinessChecker{report: serverdiagnostics.Report{
		Readiness: serverdiagnostics.Readiness{
			Ready: false, ReasonCode: "server.readiness.required_check_unhealthy",
			SafeMessage: "Required dependency is unavailable",
		},
	}}
	handler, err := NewHandler(checker)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
}

func TestReadyzHidesInternalDiagnosticsError(t *testing.T) {
	checker := &fakeReadinessChecker{err: errors.New("sensitive database connection details")}
	handler, err := NewHandler(checker)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if response.Body.String() == "" || contains(response.Body.String(), "sensitive") {
		t.Fatalf("unexpected readiness response: %s", response.Body.String())
	}
}

func TestHealthHandlerRejectsUnsupportedMethod(t *testing.T) {
	handler, err := NewHandler(&fakeReadinessChecker{})
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/healthz", nil))
	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

func contains(value, fragment string) bool {
	for i := 0; i+len(fragment) <= len(value); i++ {
		if value[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}

func TestReadyzPreservesExistingResponseShapeWhenReady(t *testing.T) {
	checker := &fakeReadinessChecker{report: serverdiagnostics.Report{
		Readiness: serverdiagnostics.Readiness{Ready: true, SafeMessage: "ready"},
		Diagnostics: platformdiagnostics.Report{Results: []platformdiagnostics.Result{
			{CheckID: "server.storage.mariadb", Observation: platformdiagnostics.Observation{Status: platformdiagnostics.StatusHealthy}},
			{CheckID: "server.storage.redis", Observation: platformdiagnostics.Observation{Status: platformdiagnostics.StatusHealthy}},
		}},
	}}
	handler, err := NewHandler(checker)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	if !contains(response.Body.String(), "ready") ||
		!contains(response.Body.String(), "mariadb") ||
		!contains(response.Body.String(), "redis") {
		t.Fatalf("unexpected readiness response: %s", response.Body.String())
	}
}

func TestReadyzFailsImmediatelyWhenDraining(t *testing.T) {
	checker := &fakeReadinessChecker{report: serverdiagnostics.Report{Readiness: serverdiagnostics.Readiness{Ready: true}}}
	handler, err := NewHandler(checker)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	handler.MarkDraining()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusServiceUnavailable)
	}
	if checker.calls != 0 {
		t.Fatalf("readiness checker calls = %d, want 0", checker.calls)
	}
}

func TestReadyzReportsRegisteredCheckStatuses(t *testing.T) {
	checker := &fakeReadinessChecker{report: serverdiagnostics.Report{
		Readiness: serverdiagnostics.Readiness{Ready: true, SafeMessage: "ready"},
		Diagnostics: platformdiagnostics.Report{
			Results: []platformdiagnostics.Result{
				{CheckID: "server.storage.mariadb", Observation: platformdiagnostics.Observation{Status: platformdiagnostics.StatusHealthy}},
				{CheckID: "server.storage.redis", Observation: platformdiagnostics.Observation{Status: platformdiagnostics.StatusHealthy}},
				{CheckID: "module.news.ingestion", Observation: platformdiagnostics.Observation{Status: platformdiagnostics.StatusHealthy}},
			},
		},
	}}
	handler, err := NewHandler(checker)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
	}
	var body struct {
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, id := range []string{"server.storage.mariadb", "server.storage.redis", "module.news.ingestion"} {
		if body.Dependencies[id] != "ok" {
			t.Fatalf("dependency %q status = %q, want ok; all=%v", id, body.Dependencies[id], body.Dependencies)
		}
	}
}
