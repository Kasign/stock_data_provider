package diagnostics

import (
	"context"
	"strings"
	"testing"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
)

func TestNewServiceComposesRequiredStorageAndModuleChecks(t *testing.T) {
	moduleCheck := check("module.self_check", true, platformdiagnostics.StatusHealthy)
	service, err := NewService(nil, []platformdiagnostics.Check{moduleCheck})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	if len(service.checks) != 3 {
		t.Fatalf("check count = %d, want 3", len(service.checks))
	}
	if service.checks[0].ID != "server.storage.mariadb" ||
		service.checks[1].ID != "server.storage.redis" ||
		service.checks[2].ID != "module.self_check" {
		t.Fatalf("unexpected check order: %q, %q, %q",
			service.checks[0].ID, service.checks[1].ID, service.checks[2].ID)
	}
}

func TestServiceRunUsesFreshRequiredResourceChecks(t *testing.T) {
	service, err := NewService(nil)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	report, err := service.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if report.Readiness.Ready {
		t.Fatal("missing shared resources must not be ready")
	}
	if report.Diagnostics.Summary.Status != platformdiagnostics.StatusUnavailable {
		t.Fatalf("summary = %s, want unavailable", report.Diagnostics.Summary.Status)
	}
}

func TestNewServiceRejectsDuplicateModuleCheckIDsAtConstruction(t *testing.T) {
	moduleCheck := check("server.storage.mariadb", false, platformdiagnostics.StatusHealthy)
	_, err := NewService(nil, []platformdiagnostics.Check{moduleCheck})
	if err == nil || !strings.Contains(err.Error(), "duplicate diagnostics check ID") {
		t.Fatalf("NewService() error = %v, want duplicate check ID error", err)
	}
}

func TestNewServiceRejectsDuplicateIDsAcrossModuleGroups(t *testing.T) {
	first := check("module.shared.check", true, platformdiagnostics.StatusHealthy)
	second := check("module.shared.check", false, platformdiagnostics.StatusHealthy)
	_, err := NewService(nil,
		[]platformdiagnostics.Check{first},
		[]platformdiagnostics.Check{second},
	)
	if err == nil || !strings.Contains(err.Error(), "module.shared.check") {
		t.Fatalf("NewService() error = %v, want duplicate module check ID", err)
	}
}
