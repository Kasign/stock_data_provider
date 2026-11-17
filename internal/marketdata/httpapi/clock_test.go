package httpapi

import (
	"testing"
	"time"
)

type httpAPIFixedClock struct {
	now time.Time
}

func (c httpAPIFixedClock) Now() time.Time {
	return c.now
}

func TestServerUsesInjectedClock(t *testing.T) {
	now := time.Date(2026, 11, 17, 16, 0, 0, 0, time.UTC)
	server := NewWithApplicationAndClock("", "forward_adjusted", ApplicationDependencies{}, nil, 0, 0, nil, httpAPIFixedClock{now: now})
	if got := server.nowUTC(); !got.Equal(now) {
		t.Fatalf("server clock = %s, want %s", got, now)
	}
}

func TestValidateIndexConstituentsAsOfQueryAtUsesChinaMarketDate(t *testing.T) {
	// UTC 16:00 已进入中国市场的次日，日期校验必须按中国市场时区解释。
	now := time.Date(2026, 11, 17, 16, 0, 0, 0, time.UTC)
	if got, message := validateIndexConstituentsAsOfQueryAt("2026-11-18", now); got != "2026-11-18" || message != "" {
		t.Fatalf("当日中国市场日期校验失败: got=%q message=%q", got, message)
	}
	if got, message := validateIndexConstituentsAsOfQueryAt("2026-11-19", now); got != "" || message == "" {
		t.Fatalf("未来中国市场日期应被拒绝: got=%q message=%q", got, message)
	}
}
