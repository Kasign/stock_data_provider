package bootstrap

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
)

func TestNewMigrationRegistryRegistersAvailableModules(t *testing.T) {
	registry, err := newMigrationRegistry()
	if err != nil {
		t.Fatalf("newMigrationRegistry() error = %v", err)
	}
	want := []string{"news", "notification", "platform.security", "research", "researchflow", "server.security_audit"}
	if got := registry.Namespaces(); !reflect.DeepEqual(got, want) {
		t.Fatalf("registered namespaces = %v, want %v", got, want)
	}
}

func TestStopConsumersThenCloseResourcesPreservesOrder(t *testing.T) {
	var events []string
	stopErr := errors.New("consumer stop failed")
	closeErr := errors.New("resource close failed")
	err := stopConsumersThenCloseResources(
		context.Background(),
		func(context.Context) error {
			events = append(events, "stop-consumers")
			return stopErr
		},
		func() error {
			events = append(events, "close-resources")
			return closeErr
		},
	)
	if !errors.Is(err, stopErr) || !errors.Is(err, closeErr) {
		t.Fatalf("shutdown error = %v, want both stop and close errors", err)
	}
	want := []string{"stop-consumers", "close-resources"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("shutdown order = %v, want %v", events, want)
	}
}

func TestJoinShutdownErrorPreservesRunAndCloseErrors(t *testing.T) {
	runErr := errors.New("HTTP server failed")
	shutdownErr := errors.New("resource close failed")
	got := joinShutdownError(runErr, shutdownErr)
	if !errors.Is(got, runErr) || !errors.Is(got, shutdownErr) {
		t.Fatalf("joined error = %v, want run and shutdown errors", got)
	}
	if got := joinShutdownError(runErr, nil); !errors.Is(got, runErr) {
		t.Fatalf("nil shutdown error changed run error: %v", got)
	}
}

func TestStopConsumersThenCloseResourcesHandlesMissingLifecycle(t *testing.T) {
	closed := false
	err := stopConsumersThenCloseResources(context.Background(), nil, func() error {
		closed = true
		return nil
	})
	if err != nil || !closed {
		t.Fatalf("shutdown without lifecycle = (%v, closed=%v), want nil and closed", err, closed)
	}
}

func TestNewDiagnosticsServiceRegistersNewsParserAndProviderConfigChecks(t *testing.T) {
	newsConfig, err := newsconfig.Load(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		switch key {
		case newsconfig.BochaEndpointEnv:
			return "https://bocha.example/v1/search", true
		case newsconfig.BochaAPIKeyEnv:
			return "test-secret-not-for-diagnostics", true
		default:
			return "", false
		}
	}))
	if err != nil {
		t.Fatalf("newsconfig.Load() error = %v", err)
	}
	service, err := newDiagnosticsService(nil, servermigration.NewRegistry(), newsConfig)
	if err != nil {
		t.Fatalf("newDiagnosticsService() error = %v", err)
	}
	report, err := service.Run(context.Background())
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	got := make(map[string]platformdiagnostics.Status, len(report.Diagnostics.Results))
	for _, result := range report.Diagnostics.Results {
		got[result.CheckID] = result.Observation.Status
	}
	for _, id := range []string{"news.parser.rss", "news.parser.atom", "news.provider.bocha.configuration"} {
		if status, exists := got[id]; !exists {
			t.Fatalf("diagnostics result %q is missing", id)
		} else if status != platformdiagnostics.StatusHealthy {
			t.Fatalf("diagnostics result %q status = %s, want healthy", id, status)
		}
	}

	for id, want := range map[string]platformdiagnostics.Status{
		"news.provider.tavily.configuration": platformdiagnostics.StatusSkipped,
		"news.provider.brave.configuration":  platformdiagnostics.StatusSkipped,
		"news.provider.searxng.configuration": platformdiagnostics.StatusSkipped,
	} {
		if status, exists := got[id]; !exists {
			t.Fatalf("diagnostics result %q is missing", id)
		} else if status != want {
			t.Fatalf("diagnostics result %q status = %s, want %s", id, status, want)
		}
	}
	for _, result := range report.Diagnostics.Results {
		if result.CheckID == "news.provider.bocha.configuration" && strings.Contains(result.Observation.SafeMessage, "test-secret-not-for-diagnostics") {
			t.Fatal("provider credential leaked into diagnostics")
		}
	}
}
