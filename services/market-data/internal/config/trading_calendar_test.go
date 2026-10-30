package config

import "testing"

func TestBaostockTradingCalendarDefaultAndEnvironmentOverrides(t *testing.T) {
	setDefaultProviderConfig(t)
	t.Setenv("MARKET_DATA_API_KEY", "test-api-key-that-is-at-least-32-characters")
	t.Setenv("MARKET_DATA_ALLOW_NO_STORAGE", "true")
	t.Setenv("MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ProviderSettings["baostock"].TradingCalendar {
		t.Fatal("BaoStock trading calendar should follow the domestic provider defaults")
	}

	t.Setenv("MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED", "false")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ProviderSettings["baostock"].TradingCalendar {
		t.Fatal("explicit false must disable trading calendar")
	}

	t.Setenv("MARKET_DATA_BAOSTOCK_TRADING_CALENDAR_ENABLED", "true")
	cfg, err = Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.ProviderSettings["baostock"].TradingCalendar {
		t.Fatal("explicit true must enable trading calendar")
	}
}
