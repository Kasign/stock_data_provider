package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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


func TestHealthLiveUsesInjectedClock(t *testing.T) {
	now := time.Date(2026, 11, 17, 16, 0, 0, 0, time.UTC)
	server := NewWithApplicationAndClock("", "forward_adjusted", ApplicationDependencies{}, nil, 0, 0, nil, httpAPIFixedClock{now: now})
	response := httptest.NewRecorder()
	server.routes(false, true).ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("health status = %d, want %d", response.Code, http.StatusOK)
	}
	var body struct {
		Time time.Time `json:"time"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if !body.Time.Equal(now) {
		t.Fatalf("health response time = %s, want %s", body.Time, now)
	}
}

func TestBarsRejectsFutureAsOfUsingInjectedClock(t *testing.T) {
	now := time.Date(2026, 11, 17, 16, 0, 0, 0, time.UTC)
	server := NewWithApplicationAndClock("", "forward_adjusted", ApplicationDependencies{}, nil, 0, 0, nil, httpAPIFixedClock{now: now})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/bars/600519.SH/daily?as_of=2026-11-17T16:00:01Z", nil)
	server.routes(false, false).ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("future as_of status = %d, want %d; body=%s", response.Code, http.StatusBadRequest, response.Body.String())
	}
}
