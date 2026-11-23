package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

func TestResearchflowHTTPHandlerRequiresOwnerSessionAndRejectsBearer(t *testing.T) {
	cookie := &http.Cookie{Name: "__Secure-owner_session", Value: strings.Repeat("s", 43), Path: "/api", HttpOnly: true}
	owner := &ownerSessionComposition{
		authenticator: fakeSessionRequestAuthenticator{cookie: cookie},
		cookie:        platformsecurity.SessionCookieConfig{Name: cookie.Name, Path: "/api"},
		researchflow: &researchflowOwnerComposition{
			createRun: &researchflowapplication.CreateRunService{},
			getRun:    &researchflowapplication.GetRunService{},
			cancelRun: &researchflowapplication.CancelRunService{},
		},
	}
	handler, err := newResearchflowHTTPHandler(owner)
	if err != nil {
		t.Fatalf("newResearchflowHTTPHandler() error = %v", err)
	}

	tests := []struct {
		name          string
		authorization string
		cookie        bool
		wantStatus    int
		wantSetCookie bool
	}{
		{name: "Owner Bearer is not accepted", authorization: "Bearer " + strings.Repeat("a", 43), wantStatus: http.StatusUnauthorized},
		{name: "ambiguous Bearer and Session are rejected", authorization: "Bearer " + strings.Repeat("a", 43), cookie: true, wantStatus: http.StatusBadRequest},
		{name: "Owner Session reaches the explicit Researchflow router", cookie: true, wantStatus: http.StatusNotFound, wantSetCookie: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/api/v1/researchflow/runs/run-1/events", nil)
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			if test.cookie {
				request.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, test.wantStatus, response.Body.String())
			}
			if got := response.Header().Get("Set-Cookie") != ""; got != test.wantSetCookie {
				t.Fatalf("Set-Cookie present = %v, want %v", got, test.wantSetCookie)
			}
		})
	}
}

func TestResearchflowAggregateResponseProjectsOnlyFrozenSchemaFields(t *testing.T) {
	now := time.Date(2026, time.November, 23, 7, 0, 0, 0, time.UTC)
	run := researchflowdomain.Run{
		Ref: "run_test", SchemaVersion: "1.0.0", Status: researchflowdomain.RunCreated, Version: 1,
		SystemAsOf: now, DeadlineAt: now.Add(30 * time.Minute), TemplateRef: "news.search",
		TemplateVersion: "1.0.0", TemplateFingerprint: strings.Repeat("a", 64),
		InputSnapshotRefs: []string{}, CorrelationID: "cor_test", CreatedAt: now,
	}
	stage := researchflowdomain.Stage{
		Ref: "stage_test", RunRef: run.Ref, Code: "news.search", Status: researchflowdomain.StagePending,
		Version: 1, Required: true, Dependencies: []string{}, RetryPolicyRef: "retry.news-search.v1",
		DiagnosticRefs: []string{}, ReasonCodes: []string{}, CreatedAt: now,
		QualityStatus: "internal-only", RiskDecision: "internal-only",
	}

	response := httptest.NewRecorder()
	(&researchflowHTTPHandler{}).writeAggregate(response, http.StatusOK, run, []researchflowdomain.Stage{stage})
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	runPayload, ok := payload["run"].(map[string]any)
	if !ok {
		t.Fatalf("run payload has type %T", payload["run"])
	}
	stagePayload := payload["stages"].([]any)[0].(map[string]any)
	if _, exists := runPayload["result_summary_ref"]; !exists || runPayload["result_summary_ref"] != nil {
		t.Fatal("non-terminal Run must expose a null result_summary_ref")
	}
	if payload["result_summary"] != nil {
		t.Fatal("non-terminal Run must expose a null top-level result_summary")
	}
	for _, field := range []string{"schema_version", "retry_at", "output_ref", "quality_summary_ref", "diagnostic_refs"} {
		if _, exists := stagePayload[field]; !exists {
			t.Fatalf("Stage response is missing required schema field %q", field)
		}
	}
	for _, field := range []string{"quality_status", "risk_decision", "attempt_no"} {
		if _, exists := stagePayload[field]; exists {
			t.Fatalf("Stage response leaked non-contract field %q", field)
		}
	}
}
