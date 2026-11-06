package diagnostics_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/diagnostics"
	"example.com/stock-analysis-flow/internal/platform/security"
)

func testSecret(value string) config.Secret {
	return config.NewEnvWithLookup(func(string) (string, bool) {
		return value, true
	}).Secret("TEST_SECRET")
}

func TestProtectedDiagnosticsRouteComposition(t *testing.T) {
	ownerToken := strings.Repeat("o", 40)
	researchToken := strings.Repeat("r", 40)
	dataToken := strings.Repeat("d", 40)
	authenticator, err := security.NewAuthenticator(testSecret(ownerToken), []security.ServiceCredential{
		{Name: "research-reader", Secret: testSecret(researchToken), Scopes: []string{security.ScopeResearchRead}},
		{Name: "data-reader", Secret: testSecret(dataToken), Scopes: []string{security.ScopeDataRead}},
	})
	if err != nil {
		t.Fatal(err)
	}

	runner := diagnostics.Runner{}
	router := security.NewRouter()
	err = router.Handle("GET /internal/diagnostics", authenticator,
		security.OwnerOrServiceAccess(security.ScopeResearchRead),
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			report, runErr := runner.Run(r.Context(), []diagnostics.Check{{
				ID:       "storage-readiness",
				Required: true,
				Run: func(context.Context) (diagnostics.Observation, error) {
					return diagnostics.Observation{Status: diagnostics.StatusHealthy, SafeMessage: "ready"}, nil
				},
			}})
			if runErr != nil {
				http.Error(w, "diagnostics failed", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(report)
		}))
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		token      string
		wantStatus int
	}{
		{name: "anonymous denied", wantStatus: http.StatusUnauthorized},
		{name: "wrong service scope denied", token: dataToken, wantStatus: http.StatusForbidden},
		{name: "research service allowed", token: researchToken, wantStatus: http.StatusOK},
		{name: "owner allowed", token: ownerToken, wantStatus: http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/internal/diagnostics", nil)
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
			}
			response := httptest.NewRecorder()
			router.Handler().ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body=%s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantStatus == http.StatusOK && !strings.Contains(response.Body.String(), "\"status\":\"healthy\"") {
				t.Fatalf("diagnostics report missing healthy observation: %s", response.Body.String())
			}
			if strings.Contains(response.Body.String(), ownerToken) ||
				strings.Contains(response.Body.String(), researchToken) ||
				strings.Contains(response.Body.String(), dataToken) {
				t.Fatalf("credential leaked in response: %s", response.Body.String())
			}
		})
	}
}
