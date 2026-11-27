package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLimitPoolCollectionTargetIsTheLatestCompletedSession(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	after := 15*time.Hour + 30*time.Minute

	cases := []struct {
		name string
		now  time.Time
		want string
	}{
		{"15:29 before close", time.Date(2026, 11, 27, 15, 29, 0, 0, location), "2026-11-26"},
		{"15:30 after close", time.Date(2026, 11, 27, 15, 30, 0, 0, location), "2026-11-27"},
		{"saturday early morning", time.Date(2026, 11, 28, 1, 0, 0, 0, location), "2026-11-27"},
		{"monday before close", time.Date(2026, 11, 30, 10, 0, 0, 0, location), "2026-11-27"},
	}
	for _, tc := range cases {
		if got := limitPoolCollectionTarget(tc.now, after); got != tc.want {
			t.Fatalf("%s: target = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestParseClockDurationRejectsMalformedValues(t *testing.T) {
	for _, bad := range []string{"24:00", "15:60", "1530", "15:3:0", "aa:bb"} {
		if _, err := parseClockDuration(bad); err == nil {
			t.Fatalf("%q must be rejected", bad)
		}
	}
	if parsed, err := parseClockDuration("15:30"); err != nil || parsed != 15*time.Hour+30*time.Minute {
		t.Fatalf("15:30 parsed to %s err=%v", parsed, err)
	}
}

// limitPoolCollectionFake 记录每次调用的 (symbol, trade_date) 并可按需失败。
type limitPoolCollectionFake struct {
	calls    []string
	failures int
}

func (fake *limitPoolCollectionFake) PersistLimitPoolFactsSnapshot(
	_ context.Context, symbol, tradeDate string, _ time.Time, _ string,
) (string, error) {
	fake.calls = append(fake.calls, symbol+"@"+tradeDate)
	if fake.failures > 0 {
		fake.failures--
		return "", errors.New("provider unavailable")
	}
	return "data_snapshot:sha256:test", nil
}

func newLimitPoolCollectionForTest(
	t *testing.T, fake *limitPoolCollectionFake, now time.Time, symbols ...string,
) *limitPoolCollectionComponent {
	t.Helper()
	component, err := newLimitPoolCollectionComponent(fake, limitPoolCollectionConfig{
		Enabled: true, Symbols: symbols, Purpose: "risk_review",
		AfterLocal: 15*time.Hour + 30*time.Minute, Interval: time.Minute, MaxAttempts: 3,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	component.now = func() time.Time { return now }
	return component
}

func TestLimitPoolCollectionRunsOncePerTradeDate(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 27, 16, 0, 0, 0, location).UTC()
	fake := &limitPoolCollectionFake{}
	component := newLimitPoolCollectionForTest(t, fake, now, "600519.SH", "600036.SH")

	component.runOnce(context.Background())
	component.runOnce(context.Background())
	component.runOnce(context.Background())

	if len(fake.calls) != 2 {
		t.Fatalf("calls = %v, want exactly one round for two symbols", fake.calls)
	}
	if component.lastSucceeded != "2026-11-27" {
		t.Fatalf("lastSucceeded = %q", component.lastSucceeded)
	}
}

// TestLimitPoolCollectionCatchesUpThePreviousSession 覆盖盘中启动：不采当天（尚无完整池），
// 而是补上最近一个已收盘交易日，这样重启也不会漏采。
func TestLimitPoolCollectionCatchesUpThePreviousSession(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 27, 14, 0, 0, 0, location)
	fake := &limitPoolCollectionFake{}
	component := newLimitPoolCollectionForTest(t, fake, now, "600519.SH")

	component.runOnce(context.Background())
	if len(fake.calls) != 1 || fake.calls[0] != "600519.SH@2026-11-26" {
		t.Fatalf("calls = %v, want the previous session 2026-11-26", fake.calls)
	}
}

func TestLimitPoolCollectionStopsAfterMaxAttempts(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 27, 16, 0, 0, 0, location).UTC()
	fake := &limitPoolCollectionFake{failures: 100}
	component := newLimitPoolCollectionForTest(t, fake, now, "600519.SH")

	for attempt := 0; attempt < 6; attempt++ {
		component.runOnce(context.Background())
	}
	if len(fake.calls) != 3 {
		t.Fatalf("calls = %d, want the configured 3 attempts", len(fake.calls))
	}
	if component.lastSucceeded != "" {
		t.Fatalf("failed collection must not be marked as succeeded: %q", component.lastSucceeded)
	}
}

func TestLimitPoolCollectionRetriesUntilSuccess(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 27, 16, 0, 0, 0, location).UTC()
	fake := &limitPoolCollectionFake{failures: 1}
	component := newLimitPoolCollectionForTest(t, fake, now, "600519.SH")

	component.runOnce(context.Background())
	if component.lastSucceeded != "" {
		t.Fatal("first attempt failed but was marked succeeded")
	}
	component.runOnce(context.Background())
	if component.lastSucceeded != "2026-11-27" || len(fake.calls) != 2 {
		t.Fatalf("retry did not converge: lastSucceeded=%q calls=%v", component.lastSucceeded, fake.calls)
	}
}

func TestLoadLimitPoolCollectionConfigDisabledWithoutSymbols(t *testing.T) {
	t.Setenv("SERVER_LIMIT_POOL_COLLECT_SYMBOLS", "")
	config, err := loadLimitPoolCollectionConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Enabled {
		t.Fatal("empty symbol list must leave the scheduler disabled")
	}
}

func TestLoadLimitPoolCollectionConfigValidatesInputs(t *testing.T) {
	t.Setenv("SERVER_LIMIT_POOL_COLLECT_SYMBOLS", "600519.SH, 600036.SH")
	t.Setenv("SERVER_LIMIT_POOL_COLLECT_AFTER", "15:45")
	t.Setenv("SERVER_LIMIT_POOL_COLLECT_INTERVAL", "10m")
	t.Setenv("SERVER_LIMIT_POOL_COLLECT_MAX_ATTEMPTS", "5")
	config, err := loadLimitPoolCollectionConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !config.Enabled || len(config.Symbols) != 2 || config.Symbols[1] != "600036.SH" {
		t.Fatalf("unexpected symbols: %+v", config)
	}
	if config.AfterLocal != 15*time.Hour+45*time.Minute || config.Interval != 10*time.Minute || config.MaxAttempts != 5 {
		t.Fatalf("unexpected schedule: %+v", config)
	}

	t.Setenv("SERVER_LIMIT_POOL_COLLECT_AFTER", "25:00")
	if _, err := loadLimitPoolCollectionConfig(); err == nil {
		t.Fatal("invalid clock must be rejected")
	}
}
