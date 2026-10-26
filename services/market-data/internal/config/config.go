package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

type ProviderSettings struct {
	Enabled         bool `json:"enabled"`
	Quote           bool `json:"quote"`
	DailyBars       bool `json:"daily_bars"`
	StockBasic      bool `json:"stock_basic"`
	SectorRankings  bool `json:"sector_rankings"`
	IndexQuotes     bool `json:"index_quotes"`
	MarketBreadth   bool `json:"market_breadth"`
	LimitPools      bool `json:"limit_pools"`
	StockBoards     bool `json:"stock_boards"`
	TradingCalendar bool `json:"trading_calendar"`
	Fundamentals    bool `json:"fundamentals"`
	Valuations      bool `json:"valuations"`
	DragonTiger     bool `json:"dragon_tiger"`
	Announcements   bool `json:"announcements"`
}

type Config struct {
	HTTPAddr                  string
	APIKey                    string
	RequestTimeout            time.Duration
	ProviderTimeout           time.Duration
	ProviderRetries           int
	CircuitFailureThreshold   int
	CircuitCooldown           time.Duration
	QuoteCacheTTL             time.Duration
	BarsCacheTTL              time.Duration
	DefaultDailyBars          int
	MaxDailyBars              int
	Adjustment                string
	ProviderPriority          []string
	ProviderSettings          map[string]ProviderSettings
	ProvidersConfigPath       string
	EastmoneyQuoteURL         string
	EastmoneyKlineURL         string
	EastmoneyStockBasicURL    string
	EastmoneySectorRankingURL string
	EastmoneyIndexURL         string
	EastmoneyMarketStocksURL  string
	EastmoneyLimitPoolBaseURL string
	EastmoneyStockBoardsURL   string
	EastmoneyDragonTigerURL   string
	EastmoneyAnnouncementsURL string
	EastmoneyUT               string
	TencentQuoteURL           string
	TencentKlineURL           string
	TencentNoneKlineURL       string
	SinaQuoteURL              string
	SinaKlineURL              string
	YahooChartURL             string
	NeteaseHistoryURL         string
	NeteaseQuoteURL           string
	TushareToken              string
	TushareAPIURL             string
	HithinkBaseURL            string
	HithinkAPIKey             string
	TickFlowAPIKey            string
	PytdxHosts                []string
	TickFlowAPIURL            string
	TickFlowFreeAPIURL        string
	BaostockHost              string
	BaostockPort              int
	BaostockUserID            string
	BaostockPassword          string
	BaostockConnectTimeout    time.Duration
	BaostockRequestTimeout    time.Duration
	MXEnabled                 bool
	MXAPIKey                  string
	MXAPIURL                  string
	MXAPITimeout              time.Duration
	QuoteRetention            time.Duration
	RedisAddr                 string
	RedisUsername             string
	RedisPassword             string
	RedisDB                   int
	MySQLDSN                  string
	AutoMigrate               bool
	AllowNoStorage            bool
}

type providerFile struct {
	ProviderPriority []string                    `json:"provider_priority"`
	AdjustmentBasis  string                      `json:"adjustment_basis"`
	Providers        map[string]ProviderSettings `json:"providers"`
}

func Load() (Config, error) {
	if err := validateEnvironment(); err != nil {
		return Config{}, err
	}
	defaults := providerFile{
		ProviderPriority: []string{"eastmoney", "baostock", "hithink", "tushare", "tickflow", "tencent", "sina", "netease", "pytdx"},
		AdjustmentBasis:  "forward_adjusted",
		Providers: map[string]ProviderSettings{
			"eastmoney": {Enabled: true, Quote: true, DailyBars: true, StockBasic: true, SectorRankings: true, IndexQuotes: true, MarketBreadth: true, LimitPools: true, StockBoards: true, DragonTiger: true, Announcements: true},
			"hithink":   {Enabled: false, Quote: false, DailyBars: false, IndexQuotes: false, TradingCalendar: false, LimitPools: false, DragonTiger: false, Fundamentals: false, Valuations: false},
			"tencent":   {Enabled: true, Quote: true, DailyBars: true},
			"baostock":  {Enabled: true, Quote: false, DailyBars: true, TradingCalendar: true, Fundamentals: true},
			"sina":      {Enabled: true, Quote: true, DailyBars: true},
			"yahoo":     {Enabled: false, Quote: false, DailyBars: false},
			"netease":   {Enabled: true, Quote: true, DailyBars: true},
			"tushare":   {Enabled: false, Quote: false, DailyBars: false},
			"tickflow":  {Enabled: true, Quote: false, DailyBars: true},
			"pytdx":     {Enabled: true, Quote: true, DailyBars: true},
		},
	}
	file := defaults
	explicitConfigPath := strings.TrimSpace(os.Getenv("MARKET_DATA_PROVIDERS_CONFIG")) != ""
	configPath := get("MARKET_DATA_PROVIDERS_CONFIG", "configs/providers.json")
	if raw, err := os.ReadFile(configPath); err == nil {
		var supplied providerFile
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&supplied); err != nil {
			return Config{}, fmt.Errorf("parse providers config %s: %w", configPath, err)
		}
		if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
			return Config{}, fmt.Errorf("parse providers config %s: expected exactly one JSON object", configPath)
		}
		for name := range supplied.Providers {
			if name != "eastmoney" && name != "baostock" && name != "tencent" && name != "sina" && name != "yahoo" && name != "netease" && name != "tushare" && name != "tickflow" && name != "pytdx" && name != "hithink" {
				return Config{}, fmt.Errorf("unknown provider %q in providers config", name)
			}
		}
		if len(supplied.ProviderPriority) > 0 {
			file.ProviderPriority = supplied.ProviderPriority
		}
		if strings.TrimSpace(supplied.AdjustmentBasis) != "" {
			file.AdjustmentBasis = supplied.AdjustmentBasis
		}
		for name, settings := range supplied.Providers {
			file.Providers[name] = settings
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return Config{}, fmt.Errorf("read providers config %s: %w", configPath, err)
	} else if explicitConfigPath {
		return Config{}, fmt.Errorf("MARKET_DATA_PROVIDERS_CONFIG was explicitly set to %q but the file does not exist", configPath)
	}

	priority := file.ProviderPriority
	if v := strings.TrimSpace(os.Getenv("MARKET_DATA_PROVIDER_PRIORITY")); v != "" {
		priority = splitList(v)
	}
	settings := make(map[string]ProviderSettings, len(file.Providers))
	for name, value := range file.Providers {
		settings[name] = value
	}
	settings["eastmoney"] = overrideProvider("EASTMONEY", settings["eastmoney"])
	settings["hithink"] = overrideProvider("HITHINK", settings["hithink"])
	settings["tencent"] = overrideProvider("TENCENT", settings["tencent"])
	settings["sina"] = overrideProvider("SINA", settings["sina"])
	settings["yahoo"] = overrideProvider("YAHOO", settings["yahoo"])
	settings["netease"] = overrideProvider("NETEASE", settings["netease"])
	settings["baostock"] = overrideProvider("BAOSTOCK", settings["baostock"])
	settings["tushare"] = overrideProvider("TUSHARE", settings["tushare"])
	settings["tickflow"] = overrideProvider("TICKFLOW", settings["tickflow"])
	settings["pytdx"] = overrideProvider("PYTDX", settings["pytdx"])
	if settings["baostock"].Quote {
		return Config{}, errors.New("BaoStock currently supports daily bars only; disable MARKET_DATA_BAOSTOCK_QUOTE_ENABLED")
	}
	cfg := Config{
		HTTPAddr:                  get("MARKET_DATA_HTTP_ADDR", ":8091"),
		APIKey:                    strings.TrimSpace(os.Getenv("MARKET_DATA_API_KEY")),
		RequestTimeout:            duration("MARKET_DATA_REQUEST_TIMEOUT", 3*time.Minute),
		ProviderTimeout:           duration("MARKET_DATA_PROVIDER_TIMEOUT", 12*time.Second),
		ProviderRetries:           integer("MARKET_DATA_PROVIDER_RETRIES", 1),
		CircuitFailureThreshold:   integer("MARKET_DATA_CIRCUIT_FAILURE_THRESHOLD", 3),
		CircuitCooldown:           duration("MARKET_DATA_CIRCUIT_COOLDOWN", 45*time.Second),
		QuoteCacheTTL:             duration("MARKET_DATA_QUOTE_CACHE_TTL", 15*time.Second),
		BarsCacheTTL:              duration("MARKET_DATA_BARS_CACHE_TTL", 10*time.Minute),
		DefaultDailyBars:          integer("MARKET_DATA_DEFAULT_DAILY_BARS", 120),
		MaxDailyBars:              integer("MARKET_DATA_MAX_DAILY_BARS", 1000),
		Adjustment:                get("MARKET_DATA_DEFAULT_ADJUSTMENT", get("MARKET_DATA_ADJUSTMENT", file.AdjustmentBasis)),
		ProviderPriority:          priority,
		ProviderSettings:          settings,
		ProvidersConfigPath:       configPath,
		EastmoneyQuoteURL:         get("MARKET_DATA_EASTMONEY_QUOTE_URL", "https://push2.eastmoney.com/api/qt/stock/get"),
		EastmoneyKlineURL:         get("MARKET_DATA_EASTMONEY_KLINE_URL", "https://push2his.eastmoney.com/api/qt/stock/kline/get"),
		EastmoneyStockBasicURL:    get("MARKET_DATA_EASTMONEY_STOCK_BASIC_URL", "https://datacenter.eastmoney.com/securities/api/data/v1/get"),
		EastmoneySectorRankingURL: get("MARKET_DATA_EASTMONEY_SECTOR_RANKING_URL", "https://push2.eastmoney.com/api/qt/clist/get"),
		EastmoneyIndexURL:         get("MARKET_DATA_EASTMONEY_INDEX_URL", "https://push2.eastmoney.com/api/qt/ulist.np/get"),
		EastmoneyMarketStocksURL:  get("MARKET_DATA_EASTMONEY_MARKET_STOCKS_URL", "https://push2ex.eastmoney.com/getTopicZDFenBu"),
		EastmoneyLimitPoolBaseURL: get("MARKET_DATA_EASTMONEY_LIMIT_POOL_BASE_URL", "https://push2ex.eastmoney.com"),
		EastmoneyStockBoardsURL:   get("MARKET_DATA_EASTMONEY_STOCK_BOARDS_URL", "https://push2.eastmoney.com/api/qt/slist/get"),
		EastmoneyDragonTigerURL:   get("MARKET_DATA_EASTMONEY_DRAGON_TIGER_URL", "https://datacenter-web.eastmoney.com/api/data/v1/get"),
		EastmoneyAnnouncementsURL: get("MARKET_DATA_EASTMONEY_ANNOUNCEMENTS_URL", "https://np-anotice-stock.eastmoney.com/api/security/ann"),
		EastmoneyUT:               get("MARKET_DATA_EASTMONEY_UT", ""),
		TencentQuoteURL:           get("MARKET_DATA_TENCENT_QUOTE_URL", "https://web.sqt.gtimg.cn/utf8/q"),
		TencentKlineURL:           get("MARKET_DATA_TENCENT_KLINE_URL", "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"),
		TencentNoneKlineURL:       get("MARKET_DATA_TENCENT_NONE_KLINE_URL", "https://web.ifzq.gtimg.cn/appstock/app/kline/kline"),
		SinaQuoteURL:              get("MARKET_DATA_SINA_QUOTE_URL", "https://hq.sinajs.cn/list="),
		SinaKlineURL:              get("MARKET_DATA_SINA_KLINE_URL", "https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData"),
		YahooChartURL:             get("MARKET_DATA_YAHOO_CHART_URL", "https://query1.finance.yahoo.com/v8/finance/chart"),
		NeteaseHistoryURL:         get("MARKET_DATA_NETEASE_HISTORY_URL", "https://quotes.money.163.com/service/chddata.html"),
		NeteaseQuoteURL:           get("MARKET_DATA_NETEASE_QUOTE_URL", "https://api.money.126.net/data/feed"),
		TushareToken:              firstNonEmpty(os.Getenv("MARKET_DATA_TUSHARE_TOKEN"), os.Getenv("TUSHARE_TOKEN")),
		TushareAPIURL:             get("MARKET_DATA_TUSHARE_API_URL", get("TUSHARE_HTTP_URL", "https://api.tushare.pro")),
		HithinkBaseURL:            get("MARKET_DATA_HITHINK_BASE_URL", "https://fuyao.aicubes.cn"),
		HithinkAPIKey:             firstNonEmpty(os.Getenv("MARKET_DATA_HITHINK_API_KEY"), os.Getenv("HITHINK_FINANCE_API_KEY")),
		TickFlowAPIKey:            firstNonEmpty(os.Getenv("MARKET_DATA_TICKFLOW_API_KEY"), os.Getenv("TICKFLOW_API_KEY")),
		TickFlowAPIURL:            get("MARKET_DATA_TICKFLOW_API_URL", get("TICKFLOW_BASE_URL", "https://api.tickflow.org")),
		TickFlowFreeAPIURL:        get("MARKET_DATA_TICKFLOW_FREE_API_URL", get("TICKFLOW_FREE_BASE_URL", "https://free-api.tickflow.org")),
		PytdxHosts:                splitList(get("MARKET_DATA_PYTDX_HOSTS", "")),
		BaostockHost:              get("MARKET_DATA_BAOSTOCK_HOST", "public-api.baostock.com"),
		BaostockPort:              integer("MARKET_DATA_BAOSTOCK_PORT", 10030),
		BaostockUserID:            get("MARKET_DATA_BAOSTOCK_USER_ID", "anonymous"),
		BaostockPassword:          get("MARKET_DATA_BAOSTOCK_PASSWORD", "123456"),
		BaostockConnectTimeout:    duration("MARKET_DATA_BAOSTOCK_CONNECT_TIMEOUT", 5*time.Second),
		BaostockRequestTimeout:    duration("MARKET_DATA_BAOSTOCK_REQUEST_TIMEOUT", 15*time.Second),
		MXEnabled:                 boolEnv("MARKET_DATA_MX_ENABLED", false),
		MXAPIKey:                  firstNonEmpty(os.Getenv("MARKET_DATA_MX_API_KEY"), os.Getenv("MX_APIKEY")),
		MXAPIURL:                  get("MARKET_DATA_MX_API_URL", "https://mkapi2.dfcfs.com/finskillshub/api/claw/query"),
		MXAPITimeout:              duration("MARKET_DATA_MX_API_TIMEOUT", 10*time.Second),
		QuoteRetention:            duration("MARKET_DATA_QUOTE_RETENTION", 30*24*time.Hour),
		RedisAddr:                 strings.TrimSpace(os.Getenv("MARKET_DATA_REDIS_ADDR")),
		RedisUsername:             strings.TrimSpace(os.Getenv("MARKET_DATA_REDIS_USERNAME")),
		RedisPassword:             os.Getenv("MARKET_DATA_REDIS_PASSWORD"),
		RedisDB:                   integer("MARKET_DATA_REDIS_DB", 0),
		AutoMigrate:               boolEnv("MARKET_DATA_DB_AUTO_MIGRATE", true),
		AllowNoStorage:            boolEnv("MARKET_DATA_ALLOW_NO_STORAGE", false),
	}
	var dsnErr error
	cfg.MySQLDSN, dsnErr = resolveMySQLDSN(cfg.AllowNoStorage)
	if dsnErr != nil {
		return cfg, dsnErr
	}
	if !validSecret(cfg.APIKey) {
		return cfg, errors.New("MARKET_DATA_API_KEY must be a non-placeholder secret with at least 32 characters")
	}
	for name, endpoint := range map[string]string{
		"MARKET_DATA_EASTMONEY_QUOTE_URL":           cfg.EastmoneyQuoteURL,
		"MARKET_DATA_EASTMONEY_KLINE_URL":           cfg.EastmoneyKlineURL,
		"MARKET_DATA_EASTMONEY_STOCK_BASIC_URL":     cfg.EastmoneyStockBasicURL,
		"MARKET_DATA_EASTMONEY_SECTOR_RANKING_URL":  cfg.EastmoneySectorRankingURL,
		"MARKET_DATA_EASTMONEY_INDEX_URL":           cfg.EastmoneyIndexURL,
		"MARKET_DATA_EASTMONEY_MARKET_STOCKS_URL":   cfg.EastmoneyMarketStocksURL,
		"MARKET_DATA_EASTMONEY_LIMIT_POOL_BASE_URL": cfg.EastmoneyLimitPoolBaseURL,
		"MARKET_DATA_EASTMONEY_STOCK_BOARDS_URL":    cfg.EastmoneyStockBoardsURL,
		"MARKET_DATA_EASTMONEY_DRAGON_TIGER_URL":    cfg.EastmoneyDragonTigerURL,
		"MARKET_DATA_EASTMONEY_ANNOUNCEMENTS_URL":   cfg.EastmoneyAnnouncementsURL,
		"MARKET_DATA_TENCENT_QUOTE_URL":             cfg.TencentQuoteURL,
		"MARKET_DATA_TENCENT_KLINE_URL":             cfg.TencentKlineURL,
		"MARKET_DATA_TENCENT_NONE_KLINE_URL":        cfg.TencentNoneKlineURL,
		"MARKET_DATA_SINA_QUOTE_URL":                cfg.SinaQuoteURL,
		"MARKET_DATA_SINA_KLINE_URL":                cfg.SinaKlineURL,
		"MARKET_DATA_YAHOO_CHART_URL":               cfg.YahooChartURL,
		"MARKET_DATA_NETEASE_HISTORY_URL":           cfg.NeteaseHistoryURL,
		"MARKET_DATA_TUSHARE_API_URL":                cfg.TushareAPIURL,
		"MARKET_DATA_HITHINK_BASE_URL":               cfg.HithinkBaseURL,
		"MARKET_DATA_TICKFLOW_API_URL":               cfg.TickFlowAPIURL,
		"MARKET_DATA_TICKFLOW_FREE_API_URL":          cfg.TickFlowFreeAPIURL,
		"MARKET_DATA_MX_API_URL":                    cfg.MXAPIURL,
	} {
		if err := validateEndpointURL(name, endpoint); err != nil {
			return cfg, err
		}
	}
	if cfg.ProviderSettings["hithink"].Enabled && strings.TrimSpace(cfg.HithinkAPIKey) == "" {
		return cfg, errors.New("MARKET_DATA_HITHINK_ENABLED=true requires MARKET_DATA_HITHINK_API_KEY or HITHINK_FINANCE_API_KEY")
	}
	if cfg.ProviderSettings["tushare"].Enabled && strings.TrimSpace(cfg.TushareToken) == "" {
		return cfg, errors.New("MARKET_DATA_TUSHARE_ENABLED=true requires MARKET_DATA_TUSHARE_TOKEN or TUSHARE_TOKEN")
	}
	if cfg.ProviderSettings["tickflow"].Quote && strings.TrimSpace(cfg.TickFlowAPIKey) == "" {
		return cfg, errors.New("MARKET_DATA_TICKFLOW_QUOTE_ENABLED=true requires MARKET_DATA_TICKFLOW_API_KEY or TICKFLOW_API_KEY")
	}
	if cfg.ProviderSettings["tushare"].Quote {
		return cfg, errors.New("Tushare currently supports daily bars only; disable MARKET_DATA_TUSHARE_QUOTE_ENABLED")
	}
	if cfg.MXEnabled && !validMiaoXiangKey(cfg.MXAPIKey) {
		return cfg, errors.New("MARKET_DATA_MX_ENABLED=true requires a non-placeholder MARKET_DATA_MX_API_KEY or MX_APIKEY")
	}
	if !cfg.AllowNoStorage {
		if cfg.MySQLDSN == "" {
			return cfg, errors.New("MARKET_DATA_DB_HOST, MARKET_DATA_DB_USER and MARKET_DATA_DB_NAME are required; the legacy MARKET_DATA_MYSQL_DSN is temporarily supported; set MARKET_DATA_ALLOW_NO_STORAGE=true only for local no-storage smoke tests")
		}
		if cfg.RedisAddr == "" {
			return cfg, errors.New("MARKET_DATA_REDIS_ADDR is required; set MARKET_DATA_ALLOW_NO_STORAGE=true only for local no-storage smoke tests")
		}
	}
	if cfg.ProviderRetries < 0 || cfg.ProviderRetries > 5 {
		return cfg, errors.New("MARKET_DATA_PROVIDER_RETRIES must be between 0 and 5")
	}
	if cfg.CircuitFailureThreshold < 1 || cfg.CircuitFailureThreshold > 100 {
		return cfg, errors.New("MARKET_DATA_CIRCUIT_FAILURE_THRESHOLD must be between 1 and 100")
	}
	if cfg.CircuitCooldown < time.Second {
		return cfg, errors.New("MARKET_DATA_CIRCUIT_COOLDOWN must be at least 1s")
	}
	if cfg.QuoteRetention < 24*time.Hour {
		return cfg, errors.New("MARKET_DATA_QUOTE_RETENTION must be at least 24h")
	}
	if cfg.MaxDailyBars < 20 || cfg.MaxDailyBars > 2000 {
		return cfg, errors.New("MARKET_DATA_MAX_DAILY_BARS must be in 20..2000")
	}
	if cfg.DefaultDailyBars < 20 || cfg.DefaultDailyBars > cfg.MaxDailyBars {
		return cfg, errors.New("MARKET_DATA_DEFAULT_DAILY_BARS must be between 20 and MARKET_DATA_MAX_DAILY_BARS")
	}
	if cfg.Adjustment != "forward_adjusted" && cfg.Adjustment != "backward_adjusted" && cfg.Adjustment != "none" {
		return cfg, fmt.Errorf("invalid adjustment basis %q", cfg.Adjustment)
	}
	if len(cfg.ProviderPriority) == 0 {
		return cfg, errors.New("provider_priority must contain at least one provider")
	}
	seen := map[string]bool{}
	quoteReady, barsReady := false, false
	quoteProviderCount, barProviderCount := 0, 0
	for _, name := range cfg.ProviderPriority {
		if name != "eastmoney" && name != "baostock" && name != "tencent" && name != "sina" && name != "yahoo" && name != "netease" && name != "tushare" && name != "tickflow" && name != "pytdx" && name != "hithink" {
			return cfg, fmt.Errorf("unknown provider %q in priority list", name)
		}
		if seen[name] {
			return cfg, fmt.Errorf("duplicate provider %q in priority list", name)
		}
		seen[name] = true
		p := cfg.ProviderSettings[name]
		if p.Enabled && p.Quote {
			quoteReady = true
			quoteProviderCount++
		}
		if p.Enabled && p.DailyBars {
			barsReady = true
			barProviderCount++
		}
	}
	if !quoteReady && !barsReady {
		return cfg, errors.New("at least one quote or daily-bar provider capability must be enabled")
	}
	quoteBudget := minimumProviderRequestBudget(cfg.ProviderTimeout, cfg.ProviderRetries, quoteProviderCount)
	barsBudget := minimumProviderRequestBudget(cfg.ProviderTimeout, cfg.ProviderRetries, barProviderCount)
	minimumRequestTimeout := quoteBudget
	if barsBudget > minimumRequestTimeout {
		minimumRequestTimeout = barsBudget
	}
	// Bundle currently fetches daily bars and quote sequentially. Its worst-case
	// budget is the sum, not the larger of the two independent endpoint budgets.
	bundleBudget := time.Duration(0)
	if quoteProviderCount > 0 && barProviderCount > 0 {
		bundleBudget = quoteBudget + barsBudget
	}
	if bundleBudget > minimumRequestTimeout {
		minimumRequestTimeout = bundleBudget
	}
	if cfg.RequestTimeout < minimumRequestTimeout {
		return cfg, fmt.Errorf("MARKET_DATA_REQUEST_TIMEOUT must be at least %s (quote=%s, daily_bars=%s, sequential_bundle=%s) for provider timeout %s and %d retries; increase the request timeout or reduce retry settings", minimumRequestTimeout, quoteBudget, barsBudget, bundleBudget, cfg.ProviderTimeout, cfg.ProviderRetries)
	}
	return cfg, nil
}

func minimumProviderRequestBudget(providerTimeout time.Duration, retries, providerCount int) time.Duration {
	if providerCount <= 0 {
		return time.Second
	}
	backoff := time.Duration(0)
	for attempt := 0; attempt < retries; attempt++ {
		delay := 100 * time.Millisecond
		for step := 0; step < attempt && delay < time.Second; step++ {
			delay *= 2
		}
		if delay > time.Second {
			delay = time.Second
		}
		backoff += delay
	}
	// Add a one-second margin for provider parsing, failover and response writing.
	return time.Duration(providerCount*(retries+1))*providerTimeout + time.Duration(providerCount)*backoff + time.Second
}

func validateEndpointURL(name, raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("%s must be an absolute http(s) endpoint URL without credentials, query, or fragment", name)
	}
	return nil
}

func overrideProvider(prefix string, p ProviderSettings) ProviderSettings {
	p.Enabled = boolEnv("MARKET_DATA_"+prefix+"_ENABLED", p.Enabled)
	p.Quote = boolEnv("MARKET_DATA_"+prefix+"_QUOTE_ENABLED", p.Quote)
	p.DailyBars = boolEnv("MARKET_DATA_"+prefix+"_DAILY_BARS_ENABLED", p.DailyBars)
	p.StockBasic = boolEnv("MARKET_DATA_"+prefix+"_STOCK_BASIC_ENABLED", p.StockBasic)
	p.SectorRankings = boolEnv("MARKET_DATA_"+prefix+"_SECTOR_RANKINGS_ENABLED", p.SectorRankings)
	p.IndexQuotes = boolEnv("MARKET_DATA_"+prefix+"_INDEX_QUOTES_ENABLED", p.IndexQuotes)
	p.MarketBreadth = boolEnv("MARKET_DATA_"+prefix+"_MARKET_BREADTH_ENABLED", p.MarketBreadth)
	p.LimitPools = boolEnv("MARKET_DATA_"+prefix+"_LIMIT_POOLS_ENABLED", p.LimitPools)
	p.StockBoards = boolEnv("MARKET_DATA_"+prefix+"_STOCK_BOARDS_ENABLED", p.StockBoards)
	p.DragonTiger = boolEnv("MARKET_DATA_"+prefix+"_DRAGON_TIGER_ENABLED", p.DragonTiger)
	p.Announcements = boolEnv("MARKET_DATA_"+prefix+"_ANNOUNCEMENTS_ENABLED", p.Announcements)
	p.TradingCalendar = boolEnv("MARKET_DATA_"+prefix+"_TRADING_CALENDAR_ENABLED", p.TradingCalendar)
	p.Fundamentals = boolEnv("MARKET_DATA_"+prefix+"_FUNDAMENTALS_ENABLED", p.Fundamentals)
	p.Valuations = boolEnv("MARKET_DATA_"+prefix+"_VALUATIONS_ENABLED", p.Valuations)
	return p
}

func splitList(s string) []string {
	out := []string{}
	for _, v := range strings.Split(s, ",") {
		v = strings.ToLower(strings.TrimSpace(v))
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}
func validMiaoXiangKey(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 8 {
		return false
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"replace-with", "change-me", "changeme", "your-key", "example-key", "placeholder"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}

// resolveMySQLDSN builds the driver DSN from separate service-specific settings.
// The legacy DSN is accepted temporarily for existing deployments, but new
// configurations should use MARKET_DATA_DB_HOST/PORT/USER/PASSWORD/NAME.
func resolveMySQLDSN(allowNoStorage bool) (string, error) {
	host := strings.TrimSpace(os.Getenv("MARKET_DATA_DB_HOST"))
	portText := strings.TrimSpace(os.Getenv("MARKET_DATA_DB_PORT"))
	user := strings.TrimSpace(os.Getenv("MARKET_DATA_DB_USER"))
	password := os.Getenv("MARKET_DATA_DB_PASSWORD")
	database := strings.TrimSpace(os.Getenv("MARKET_DATA_DB_NAME"))
	charset := strings.TrimSpace(os.Getenv("MARKET_DATA_DB_CHARSET"))
	timezone := strings.TrimSpace(os.Getenv("MARKET_DATA_DB_TIMEZONE"))

	hasSeparatedConfig := host != "" || portText != "" || user != "" || password != "" || database != "" || charset != "" || timezone != ""
	if !hasSeparatedConfig {
		if legacyDSN := strings.TrimSpace(os.Getenv("MARKET_DATA_MYSQL_DSN")); legacyDSN != "" {
			return legacyDSN, nil
		}
		if allowNoStorage {
			return "", nil
		}
		return "", errors.New("MARKET_DATA_DB_HOST, MARKET_DATA_DB_USER and MARKET_DATA_DB_NAME are required")
	}

	if host == "" || user == "" || database == "" {
		return "", errors.New("MARKET_DATA_DB_HOST, MARKET_DATA_DB_USER and MARKET_DATA_DB_NAME must not be empty")
	}
	port := 3306
	if portText != "" {
		parsed, err := strconv.Atoi(portText)
		if err != nil || parsed < 1 || parsed > 65535 {
			return "", errors.New("MARKET_DATA_DB_PORT must be an integer between 1 and 65535")
		}
		port = parsed
	}
	if charset == "" {
		charset = "utf8mb4"
	}
	if timezone == "" {
		timezone = "UTC"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return "", fmt.Errorf("MARKET_DATA_DB_TIMEZONE must be a valid IANA timezone: %w", err)
	}

	// Start from the driver's defaults. A zero-value Config disables
	// AllowNativePasswords and breaks MariaDB users using mysql_native_password.
	driverConfig := mysqlDriver.NewConfig()
	driverConfig.User = user
	driverConfig.Passwd = password
	driverConfig.Net = "tcp"
	driverConfig.Addr = net.JoinHostPort(host, strconv.Itoa(port))
	driverConfig.DBName = database
	driverConfig.Params = map[string]string{"charset": charset}
	driverConfig.ParseTime = true
	driverConfig.Loc = location
	return driverConfig.FormatDSN(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func get(k, d string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return d
}
func duration(k string, d time.Duration) time.Duration {
	if s := strings.TrimSpace(os.Getenv(k)); s != "" {
		v, e := time.ParseDuration(s)
		if e != nil || v <= 0 {
			return d
		}
		return v
	}
	return d
}
func integer(k string, d int) int {
	if s := strings.TrimSpace(os.Getenv(k)); s != "" {
		v, e := strconv.Atoi(s)
		if e == nil {
			return v
		}
	}
	return d
}
func boolEnv(k string, d bool) bool {
	if s := strings.TrimSpace(os.Getenv(k)); s != "" {
		v, e := strconv.ParseBool(s)
		if e == nil {
			return v
		}
	}
	return d
}

func validateEnvironment() error {
	for _, key := range []string{
		"MARKET_DATA_REQUEST_TIMEOUT", "MARKET_DATA_PROVIDER_TIMEOUT",
		"MARKET_DATA_BAOSTOCK_CONNECT_TIMEOUT", "MARKET_DATA_BAOSTOCK_REQUEST_TIMEOUT",
		"MARKET_DATA_CIRCUIT_COOLDOWN", "MARKET_DATA_QUOTE_CACHE_TTL", "MARKET_DATA_BARS_CACHE_TTL", "MARKET_DATA_QUOTE_RETENTION",
	} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			d, err := time.ParseDuration(value)
			if err != nil || d < time.Millisecond {
				return fmt.Errorf("%s must be a Go duration of at least 1ms, got %q", key, value)
			}
		}
	}
	for _, key := range []string{
		"MARKET_DATA_DEFAULT_DAILY_BARS", "MARKET_DATA_MAX_DAILY_BARS", "MARKET_DATA_REDIS_DB",
		"MARKET_DATA_BAOSTOCK_PORT",
		"MARKET_DATA_PROVIDER_RETRIES", "MARKET_DATA_CIRCUIT_FAILURE_THRESHOLD",
	} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			n, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("%s must be an integer, got %q", key, value)
			}
			if key == "MARKET_DATA_REDIS_DB" && n < 0 {
				return errors.New("MARKET_DATA_REDIS_DB must not be negative")
			}
			if key == "MARKET_DATA_BAOSTOCK_PORT" && (n < 1 || n > 65535) {
				return errors.New("MARKET_DATA_BAOSTOCK_PORT must be an integer between 1 and 65535")
			}
			if key == "MARKET_DATA_PROVIDER_RETRIES" && (n < 0 || n > 5) {
				return errors.New("MARKET_DATA_PROVIDER_RETRIES must be between 0 and 5")
			}
			if key == "MARKET_DATA_CIRCUIT_FAILURE_THRESHOLD" && (n < 1 || n > 100) {
				return errors.New("MARKET_DATA_CIRCUIT_FAILURE_THRESHOLD must be between 1 and 100")
			}
		}
	}
	for _, key := range []string{
		"MARKET_DATA_DB_AUTO_MIGRATE", "MARKET_DATA_EASTMONEY_ENABLED",
		"MARKET_DATA_BAOSTOCK_ENABLED", "MARKET_DATA_BAOSTOCK_QUOTE_ENABLED", "MARKET_DATA_BAOSTOCK_DAILY_BARS_ENABLED",
		"MARKET_DATA_EASTMONEY_QUOTE_ENABLED", "MARKET_DATA_EASTMONEY_DAILY_BARS_ENABLED",
		"MARKET_DATA_EASTMONEY_STOCK_BASIC_ENABLED", "MARKET_DATA_EASTMONEY_SECTOR_RANKINGS_ENABLED",
		"MARKET_DATA_EASTMONEY_INDEX_QUOTES_ENABLED", "MARKET_DATA_EASTMONEY_MARKET_BREADTH_ENABLED",
		"MARKET_DATA_EASTMONEY_LIMIT_POOLS_ENABLED", "MARKET_DATA_EASTMONEY_STOCK_BOARDS_ENABLED", "MARKET_DATA_EASTMONEY_DRAGON_TIGER_ENABLED", "MARKET_DATA_EASTMONEY_ANNOUNCEMENTS_ENABLED",
		"MARKET_DATA_TENCENT_ENABLED", "MARKET_DATA_TENCENT_QUOTE_ENABLED",
		"MARKET_DATA_TENCENT_DAILY_BARS_ENABLED", "MARKET_DATA_SINA_ENABLED", "MARKET_DATA_MX_ENABLED",
		"MARKET_DATA_YAHOO_ENABLED", "MARKET_DATA_YAHOO_QUOTE_ENABLED", "MARKET_DATA_YAHOO_DAILY_BARS_ENABLED",
		"MARKET_DATA_NETEASE_ENABLED", "MARKET_DATA_NETEASE_QUOTE_ENABLED", "MARKET_DATA_NETEASE_DAILY_BARS_ENABLED",
		"MARKET_DATA_TUSHARE_ENABLED", "MARKET_DATA_TUSHARE_QUOTE_ENABLED", "MARKET_DATA_TUSHARE_DAILY_BARS_ENABLED",
		"MARKET_DATA_TICKFLOW_ENABLED", "MARKET_DATA_TICKFLOW_QUOTE_ENABLED", "MARKET_DATA_TICKFLOW_DAILY_BARS_ENABLED",
		"MARKET_DATA_HITHINK_ENABLED", "MARKET_DATA_HITHINK_QUOTE_ENABLED", "MARKET_DATA_HITHINK_DAILY_BARS_ENABLED", "MARKET_DATA_HITHINK_FUNDAMENTALS_ENABLED", "MARKET_DATA_HITHINK_INDEX_QUOTES_ENABLED", "MARKET_DATA_HITHINK_TRADING_CALENDAR_ENABLED", "MARKET_DATA_HITHINK_LIMIT_POOLS_ENABLED", "MARKET_DATA_HITHINK_DRAGON_TIGER_ENABLED", "MARKET_DATA_HITHINK_VALUATIONS_ENABLED",
		"MARKET_DATA_PYTDX_ENABLED", "MARKET_DATA_PYTDX_QUOTE_ENABLED", "MARKET_DATA_PYTDX_DAILY_BARS_ENABLED",
		"MARKET_DATA_SINA_QUOTE_ENABLED", "MARKET_DATA_SINA_DAILY_BARS_ENABLED", "MARKET_DATA_ALLOW_NO_STORAGE",
	} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			if _, err := strconv.ParseBool(value); err != nil {
				return fmt.Errorf("%s must be a boolean, got %q", key, value)
			}
		}
	}
	return nil
}

func validSecret(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < 32 {
		return false
	}
	lower := strings.ToLower(value)
	for _, marker := range []string{"replace-with", "change-me", "changeme", "your-secret", "example-secret"} {
		if strings.Contains(lower, marker) {
			return false
		}
	}
	return true
}
