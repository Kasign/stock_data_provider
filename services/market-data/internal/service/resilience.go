package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// ResilienceConfig governs provider retries and per-capability circuit breakers.
type ResilienceConfig struct {
	Retries                 int
	CircuitFailureThreshold int
	CircuitCooldown         time.Duration
}

func (c ResilienceConfig) normalized() ResilienceConfig {
	if c.Retries < 0 {
		c.Retries = 0
	}
	if c.Retries > 5 {
		c.Retries = 5
	}
	if c.CircuitFailureThreshold <= 0 {
		c.CircuitFailureThreshold = 3
	}
	if c.CircuitCooldown < time.Second {
		c.CircuitCooldown = 45 * time.Second
	}
	return c
}

type breakerState struct {
	failures      int
	openUntil     time.Time
	probeInFlight bool
	lastSuccessAt time.Time
	lastFailureAt time.Time
	totalSuccess  uint64
	totalFailure  uint64
}

// CircuitDiagnostic 是熔断器基于真实业务调用维护的只读运行状态。
type CircuitDiagnostic struct {
	Capability       string     `json:"capability"`
	Status           string     `json:"status"`
	ConsecutiveFails int        `json:"consecutive_failures"`
	TotalSuccess     uint64     `json:"total_success"`
	TotalFailure     uint64     `json:"total_failure"`
	LastSuccessAt    *time.Time `json:"last_success_at"`
	LastFailureAt    *time.Time `json:"last_failure_at"`
	OpenUntil        *time.Time `json:"open_until"`
}

type circuitBreaker struct {
	mu        sync.Mutex
	states    map[string]*breakerState
	threshold int
	cooldown  time.Duration
	now       func() time.Time
}

func newCircuitBreaker(threshold int, cooldown time.Duration) *circuitBreaker {
	cfg := (ResilienceConfig{CircuitFailureThreshold: threshold, CircuitCooldown: cooldown}).normalized()
	return &circuitBreaker{states: make(map[string]*breakerState), threshold: cfg.CircuitFailureThreshold, cooldown: cfg.CircuitCooldown, now: time.Now}
}

func (b *circuitBreaker) state(key string) *breakerState {
	state := b.states[key]
	if state == nil {
		state = &breakerState{}
		b.states[key] = state
	}
	return state
}

// Allow admits normal requests, blocks an open circuit, and admits one probe
// after the cooldown. Keys include provider and capability so a broken quote
// endpoint does not disable a working daily-history endpoint.
func (b *circuitBreaker) Allow(key string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.state(key)
	if state.openUntil.IsZero() {
		return true
	}
	now := b.now()
	if now.Before(state.openUntil) {
		return false
	}
	if state.probeInFlight {
		return false
	}
	state.probeInFlight = true
	return true
}

func (b *circuitBreaker) Success(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.state(key)
	state.failures = 0
	state.openUntil = time.Time{}
	state.probeInFlight = false
	state.lastSuccessAt = b.now().UTC()
	state.totalSuccess++
}

// Abandon releases a half-open probe when the caller cancels the request. Caller
// cancellation is not a provider health result and must neither close nor reopen
// the circuit, but it must not leave probeInFlight stuck forever.
func (b *circuitBreaker) Abandon(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if state := b.states[key]; state != nil && state.probeInFlight {
		state.probeInFlight = false
	}
}

func (b *circuitBreaker) Failure(key string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	state := b.state(key)
	now := b.now()
	state.lastFailureAt = now.UTC()
	state.totalFailure++
	if state.probeInFlight {
		state.failures = b.threshold
		state.openUntil = now.Add(b.cooldown)
		state.probeInFlight = false
		return
	}
	if !state.openUntil.IsZero() {
		return // Already open; concurrent in-flight failures do not extend cooldown.
	}
	state.failures++
	if state.failures >= b.threshold {
		state.openUntil = now.Add(b.cooldown)
	}
}

func retryDelay(attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	// Bounded exponential backoff: 100ms, 200ms, 400ms, 800ms, then 1s.
	d := 100 * time.Millisecond * time.Duration(math.Pow(2, float64(attempt)))
	if d > time.Second {
		return time.Second
	}
	return d
}

func waitRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(retryDelay(attempt))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func retryQuote(ctx context.Context, retries int, fetch func(context.Context) (*model.Quote, error)) (*model.Quote, error) {
	var quote *model.Quote
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		quote, err = fetch(ctx)
		if err == nil && quote == nil {
			err = errors.New("provider returned an empty quote")
		}
		if err == nil || ctx.Err() != nil || attempt == retries {
			break
		}
		if waitErr := waitRetry(ctx, attempt); waitErr != nil {
			return nil, waitErr
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("provider call failed after %d attempt(s): %w", retries+1, err)
	}
	return quote, nil
}

func retryBars(ctx context.Context, retries int, fetch func(context.Context) ([]model.DailyBar, error)) ([]model.DailyBar, error) {
	var bars []model.DailyBar
	var err error
	for attempt := 0; attempt <= retries; attempt++ {
		bars, err = fetch(ctx)
		if err == nil && len(bars) == 0 {
			err = errors.New("provider returned an empty daily-bar response")
		}
		if err == nil || ctx.Err() != nil || attempt == retries {
			break
		}
		if waitErr := waitRetry(ctx, attempt); waitErr != nil {
			return nil, waitErr
		}
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("provider call failed after %d attempt(s): %w", retries+1, err)
	}
	return bars, nil
}

// Diagnostics 返回指定 Provider 能力的熔断状态快照，不触发任何 Provider 请求。
func (b *circuitBreaker) Diagnostics(keys map[string]string) []CircuitDiagnostic {
	b.mu.Lock()
	defer b.mu.Unlock()
	result := make([]CircuitDiagnostic, 0, len(keys))
	now := b.now()
	for key, capability := range keys {
		state := b.states[key]
		item := CircuitDiagnostic{Capability: capability, Status: "not_observed"}
		if state != nil {
			item.ConsecutiveFails = state.failures
			item.TotalSuccess = state.totalSuccess
			item.TotalFailure = state.totalFailure
			if !state.lastSuccessAt.IsZero() {
				value := state.lastSuccessAt
				item.LastSuccessAt = &value
			}
			if !state.lastFailureAt.IsZero() {
				value := state.lastFailureAt
				item.LastFailureAt = &value
			}
			if !state.openUntil.IsZero() {
				if now.Before(state.openUntil) {
					item.Status = "open"
					value := state.openUntil.UTC()
					item.OpenUntil = &value
				} else if state.probeInFlight {
					item.Status = "half_open"
				} else {
					item.Status = "half_open"
				}
			} else if state.totalFailure > 0 && state.lastFailureAt.After(state.lastSuccessAt) {
				item.Status = "degraded"
			} else if state.totalSuccess > 0 {
				item.Status = "healthy"
			}
		}
		result = append(result, item)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Capability < result[j].Capability })
	return result
}
