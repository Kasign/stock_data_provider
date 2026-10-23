package config

import "testing"

func TestBaostockTradingCalendarIsOptIn(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderSettings["baostock"].TradingCalendar {
		t.Fatal("BaoStock trading calendar must be disabled by default")
	}
	t.Setenv("MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ProviderSettings["baostock"].TradingCalendar {
		t.Fatal("explicit environment override should enable trading calendar")
	}
}}
	if cfg.ProviderSettings["baostock"].TradingCalendar { t.Fatal("BaoStock trading calendar must be disabled by default") }
	t.Setenv("MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED", "true")
	cfg, err = Load()
	if err != nil { t.Fatal(err) }
	if !cfg.ProviderSettings["baostock"].TradingCalendar { t.Fatal("explicit environment override should enable trading calendar") }
}
