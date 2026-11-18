package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	marketdataconfig "example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	marketdatatransport "example.com/stock-analysis-flow/internal/marketdata/transport"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	"example.com/stock-analysis-flow/internal/server/resources"

	"github.com/injoyai/tdx"
)

// newMarketDataDependencies 将 Root 持有的 Platform HTTP Client 和受控 TCP
// 拨号器注入 Data；Data Provider 不自行创建公共出站客户端。
func newMarketDataDependencies(shared *resources.Resources, cfg marketdataconfig.Config) (marketdatabootstrap.Dependencies, error) {
	if shared == nil || shared.HTTPClient == nil {
		return marketdatabootstrap.Dependencies{}, errors.New("Root Platform HTTP Client is required by Market Data")
	}
	deps := marketdatabootstrap.Dependencies{
		RequireEgressDependencies: true,
		Logger:                    slog.Default(),
		EastmoneyHTTPDoers:        make(map[string]provider.HTTPDoer),
	}

	bind := func(raw, dependency string) (*platformhttpclient.BoundDoer, error) {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u == nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") {
			return nil, fmt.Errorf("invalid Market Data endpoint for %s", dependency)
		}
		port := uint16(443)
		if u.Scheme == "http" {
			port = 80
		}
		if explicit := u.Port(); explicit != "" {
			parsed, parseErr := strconv.ParseUint(explicit, 10, 16)
			if parseErr != nil || parsed == 0 {
				return nil, fmt.Errorf("invalid Market Data endpoint port for %s", dependency)
			}
			port = uint16(parsed)
		}
		return shared.BindHTTPDestination(platformhttpclient.DestinationPolicy{
			Dependency: dependency, AllowedHosts: []string{strings.ToLower(u.Hostname())},
			AllowedPorts: []uint16{port}, AllowHTTP: u.Scheme == "http",
			AllowRedirects: false, Idempotent: true,
		})
	}
	bindOne := func(raw, dependency string) (provider.HTTPDoer, error) {
		doer, err := bind(raw, dependency)
		if err != nil {
			return nil, err
		}
		return doer, nil
	}

	eastmoneyEndpoints := []string{
		cfg.EastmoneyQuoteURL, cfg.EastmoneyKlineURL, cfg.EastmoneyStockBasicURL,
		cfg.EastmoneySectorRankingURL, cfg.EastmoneyIndexURL, cfg.EastmoneyMarketStocksURL,
		cfg.EastmoneyLimitPoolBaseURL, cfg.EastmoneyStockBoardsURL, cfg.EastmoneyDragonTigerURL,
		cfg.EastmoneyAnnouncementsURL, cfg.EastmoneyHotRankingsURL, cfg.EastmoneyHotSurgeURL,
		cfg.EastmoneyCapitalFlowURL,
	}
	if marketDataProviderActive(cfg, "eastmoney") {
		for _, endpoint := range eastmoneyEndpoints {
			if strings.TrimSpace(endpoint) == "" {
				continue
			}
			doer, err := bindOne(endpoint, "market_data_eastmoney")
			if err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
			origin, err := endpointOrigin(endpoint)
			if err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
			deps.EastmoneyHTTPDoers[origin] = doer
		}
	}

	var err error
	settings := cfg.ProviderSettings
	if marketDataProviderActive(cfg, "hithink") {
		if deps.HithinkHTTPDoer, err = bindOne(cfg.HithinkBaseURL, "market_data_hithink"); err != nil {
			return marketdatabootstrap.Dependencies{}, err
		}
	}
	if marketDataProviderActive(cfg, "tencent") {
		if settings["tencent"].Quote {
			if deps.TencentQuoteHTTPDoer, err = bindOne(cfg.TencentQuoteURL, "market_data_tencent"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
		if settings["tencent"].DailyBars {
			if deps.TencentKlineHTTPDoer, err = bindOne(cfg.TencentKlineURL, "market_data_tencent"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
			if deps.TencentNoneKlineHTTPDoer, err = bindOne(cfg.TencentNoneKlineURL, "market_data_tencent"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
		if settings["tencent"].StockSearch {
			if deps.TencentSearchHTTPDoer, err = bindOne(cfg.TencentStockSearchURL, "market_data_tencent"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
	}
	if marketDataProviderActive(cfg, "sina") {
		if settings["sina"].Quote {
			if deps.SinaQuoteHTTPDoer, err = bindOne(cfg.SinaQuoteURL, "market_data_sina"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
		if settings["sina"].DailyBars {
			if deps.SinaKlineHTTPDoer, err = bindOne(cfg.SinaKlineURL, "market_data_sina"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
	}
	if marketDataProviderActive(cfg, "yahoo") {
		if deps.YahooHTTPDoer, err = bindOne(cfg.YahooChartURL, "market_data_yahoo"); err != nil {
			return marketdatabootstrap.Dependencies{}, err
		}
	}
	if marketDataProviderActive(cfg, "netease") {
		if settings["netease"].Quote {
			if deps.NeteaseQuoteHTTPDoer, err = bindOne(cfg.NeteaseQuoteURL, "market_data_netease"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
		if settings["netease"].DailyBars {
			if deps.NeteaseHistoryHTTPDoer, err = bindOne(cfg.NeteaseHistoryURL, "market_data_netease"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
	}
	if marketDataProviderActive(cfg, "tushare") {
		if deps.TushareHTTPDoer, err = bindOne(cfg.TushareAPIURL, "market_data_tushare"); err != nil {
			return marketdatabootstrap.Dependencies{}, err
		}
	}
	if marketDataProviderActive(cfg, "tickflow") {
		tickflowSettings := settings["tickflow"]
		if tickflowSettings.Quote || tickflowSettings.IndexQuotes || tickflowSettings.MarketBreadth ||
			tickflowSettings.SectorRankings || (tickflowSettings.DailyBars && strings.TrimSpace(cfg.TickFlowAPIKey) != "") {
			if deps.TickFlowAuthenticatedHTTPDoer, err = bindOne(cfg.TickFlowAPIURL, "market_data_tickflow"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
		if tickflowSettings.DailyBars && strings.TrimSpace(cfg.TickFlowAPIKey) == "" {
			if deps.TickFlowPublicHTTPDoer, err = bindOne(cfg.TickFlowFreeAPIURL, "market_data_tickflow_public"); err != nil {
				return marketdatabootstrap.Dependencies{}, err
			}
		}
	}
	if marketDataProviderActive(cfg, "pytdx") {
		pytdxHosts := append([]string(nil), cfg.PytdxHosts...)
		if len(pytdxHosts) == 0 {
			pytdxHosts = append(pytdxHosts, tdx.Hosts...)
		}
		deps.PytdxDialContext, err = newMarketDataTCPDialer(pytdxHosts)
		if err != nil {
			return marketdatabootstrap.Dependencies{}, fmt.Errorf("configure Pytdx controlled TCP egress: %w", err)
		}
	}
	if marketDataProviderActive(cfg, "baostock") {
		baostockAddress := net.JoinHostPort(strings.Trim(cfg.BaostockHost, "[]"), strconv.Itoa(cfg.BaostockPort))
		deps.BaoStockDialContext, err = newMarketDataTCPDialer([]string{baostockAddress})
		if err != nil {
			return marketdatabootstrap.Dependencies{}, fmt.Errorf("configure BaoStock controlled TCP egress: %w", err)
		}
	}
	if marketDataProviderActive(cfg, "xueqiu") {
		if deps.XueqiuHTTPDoer, err = bindOne(cfg.XueqiuBaseURL, "market_data_xueqiu"); err != nil {
			return marketdatabootstrap.Dependencies{}, err
		}
	}
	if cfg.MXEnabled {
		if deps.MiaoXiangHTTPDoer, err = bindOne(cfg.MXAPIURL, "market_data_miaoxiang"); err != nil {
			return marketdatabootstrap.Dependencies{}, err
		}
	}

	return deps, nil
}

func endpointOrigin(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil || u.Host == "" {
		return "", fmt.Errorf("invalid Market Data endpoint origin")
	}
	return strings.ToLower(u.Scheme + "://" + u.Host), nil
}

func newMarketDataTCPDialer(configured []string) (marketdatatransport.DialContextFunc, error) {
	allowed := make(map[string]struct{}, len(configured))
	for _, raw := range configured {
		address, err := normalizeMarketDataTCPAddress(raw)
		if err != nil {
			return nil, err
		}
		allowed[address] = struct{}{}
	}
	if len(allowed) == 0 {
		return nil, errors.New("at least one configured TCP endpoint is required")
	}
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" && network != "tcp4" && network != "tcp6" {
			return nil, errors.New("Market Data TCP egress only permits TCP")
		}
		canonical, err := normalizeMarketDataTCPAddress(address)
		if err != nil {
			return nil, err
		}
		if _, ok := allowed[canonical]; !ok {
			return nil, fmt.Errorf("Market Data TCP destination %q is not allowlisted", canonical)
		}
		host, port, _ := net.SplitHostPort(canonical)
		ips, err := resolvePublicMarketDataIPs(ctx, host)
		if err != nil {
			return nil, err
		}
		dialer := &net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
		var lastErr error
		for _, ip := range ips {
			target := net.JoinHostPort(ip.String(), port)
			conn, dialErr := dialer.DialContext(ctx, network, target)
			if dialErr == nil {
				return conn, nil
			}
			lastErr = dialErr
		}
		if lastErr == nil {
			lastErr = errors.New("no public IP address resolved for Market Data TCP destination")
		}
		return nil, lastErr
	}, nil
}

func normalizeMarketDataTCPAddress(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("Market Data TCP destination is empty")
	}
	host, port, err := net.SplitHostPort(raw)
	if err != nil {
		host = strings.Trim(strings.TrimSpace(raw), "[]")
		port = "7709"
		if host == "" {
			return "", errors.New("Market Data TCP destination host is empty")
		}
	}
	parsed, err := strconv.ParseUint(port, 10, 16)
	if err != nil || parsed == 0 {
		return "", errors.New("Market Data TCP destination port is invalid")
	}
	return net.JoinHostPort(strings.ToLower(host), strconv.FormatUint(parsed, 10)), nil
}

func resolvePublicMarketDataIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	if literal, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		literal = literal.Unmap()
		if !isPublicMarketDataIP(literal) {
			return nil, errors.New("Market Data TCP destination resolved to a non-public IP")
		}
		return []netip.Addr{literal}, nil
	}
	resolved, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve Market Data TCP destination: %w", err)
	}
	if len(resolved) == 0 {
		return nil, errors.New("Market Data TCP destination resolved to no IP addresses")
	}
	ips := make([]netip.Addr, 0, len(resolved))
	for _, ip := range resolved {
		ip = ip.Unmap()
		if !isPublicMarketDataIP(ip) {
			return nil, errors.New("Market Data TCP destination resolved to a non-public IP")
		}
		ips = append(ips, ip)
	}
	return ips, nil
}

func isPublicMarketDataIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Is4In6() || !ip.IsGlobalUnicast() ||
		ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	blocked := []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("240.0.0.0/4"),
		netip.MustParsePrefix("2001:db8::/32"),
		netip.MustParsePrefix("fc00::/7"),
		netip.MustParsePrefix("fe80::/10"),
		netip.MustParsePrefix("ff00::/8"),
	}
	for _, prefix := range blocked {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

// marketDataProviderActive 只为启用且声明了业务能力的 Provider 创建受控出口。
func marketDataProviderActive(cfg marketdataconfig.Config, name string) bool {
	settings, ok := cfg.ProviderSettings[name]
	if !ok || !settings.Enabled {
		return false
	}
	return settings.Quote || settings.DailyBars || settings.StockBasic || settings.StockSearch ||
		settings.SectorRankings || settings.IndexQuotes || settings.MarketBreadth ||
		settings.LimitPools || settings.StockBoards || settings.TradingCalendar ||
		settings.SecurityTradingStatus || settings.Fundamentals || settings.Valuations ||
		settings.IndexCatalog || settings.ChipDistribution || settings.CapitalFlow ||
		settings.MarketCapitalFlow || settings.HotRankings || settings.DragonTiger ||
		settings.Announcements
}
