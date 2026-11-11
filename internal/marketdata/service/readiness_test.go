package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/platform/fault"
)

type readinessTestCache struct {
	err error
}

func (c readinessTestCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}
func (c readinessTestCache) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (c readinessTestCache) Ping(context.Context) error                               { return c.err }
func (c readinessTestCache) Enabled() bool                                            { return true }

func TestReadyUsesPlatformDiagnosticsAndKeepsDependencyFault(t *testing.T) {
	s := &DataService{cache: readinessTestCache{err: errors.New("connection refused")}}
	err := s.Ready(context.Background())
	var got *fault.Fault
	if !errors.As(err, &got) {
		t.Fatalf("readiness failure must return platform Fault, got %T %v", err, err)
	}
	if got.Class != fault.ClassUnavailable || got.Code != "market_data.redis_unavailable" ||
		got.Dependency != "redis" || got.SafeMessage != "Redis is unavailable" {
		t.Fatalf("unexpected readiness fault: %+v", got)
	}
}

func TestReadySkipsDisabledCacheAndMissingOptionalStore(t *testing.T) {
	s := &DataService{cache: cache.NewDisabled()}
	if err := s.Ready(context.Background()); err != nil {
		t.Fatalf("disabled cache and absent optional storage should remain ready: %v", err)
	}
}

func TestReadySkipsUnconfiguredStore(t *testing.T) {
	s := &DataService{cache: cache.NewDisabled(), store: &repository.Store{}}
	if err := s.Ready(context.Background()); err != nil {
		t.Fatalf("unconfigured optional Store should be skipped: %v", err)
	}
}

func TestReadyRejectsNilContext(t *testing.T) {
	s := &DataService{cache: cache.NewDisabled()}
	if err := s.Ready(nil); err == nil {
		t.Fatal("nil readiness context must be rejected")
	}
}
