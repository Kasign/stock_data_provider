package bootstrap

import (
	"fmt"

	"example.com/stock-analysis-flow/internal/marketdata/config"
)

// validateEgressDependencies 仅校验实际启用 Provider 的出站注入是否完整。
// 具体 BoundDoer/BoundDialer 的目标地址、DNS/IP 和重定向策略由 Root 负责。
func validateEgressDependencies(cfg config.Config, deps Dependencies) error {
	for _, name := range cfg.ProviderPriority {
		if !cfg.ProviderSettings[name].Enabled {
			continue
		}
		if name == "eastmoney" {
			origins, err := newEastmoneyProvider(cfg).HTTPOrigins()
			if err != nil {
				return fmt.Errorf("controlled egress for eastmoney: %w", err)
			}
			for _, origin := range origins {
				if deps.EastmoneyHTTPDoers[origin] == nil {
					return fmt.Errorf("controlled egress dependency missing: eastmoney origin %q", origin)
				}
			}
			continue
		}
		switch name {
		case "hithink":
			if deps.HithinkHTTPDoer == nil { return missingEgress("hithink HTTP") }
		case "baostock":
			if deps.BaoStockDialContext == nil { return missingEgress("baostock TCP") }
		case "tencent":
			if deps.TencentQuoteHTTPDoer == nil || deps.TencentKlineHTTPDoer == nil ||
				deps.TencentNoneKlineHTTPDoer == nil || deps.TencentSearchHTTPDoer == nil {
				return missingEgress("tencent HTTP endpoints")
			}
		case "sina":
			if deps.SinaQuoteHTTPDoer == nil || deps.SinaKlineHTTPDoer == nil {
				return missingEgress("sina HTTP endpoints")
			}
		case "yahoo":
			if deps.YahooHTTPDoer == nil { return missingEgress("yahoo HTTP") }
		case "netease":
			if deps.NeteaseQuoteHTTPDoer == nil || deps.NeteaseHistoryHTTPDoer == nil {
				return missingEgress("netease HTTP endpoints")
			}
		case "tushare":
			if deps.TushareHTTPDoer == nil { return missingEgress("tushare HTTP") }
		case "tickflow":
			if deps.TickFlowAuthenticatedHTTPDoer == nil || deps.TickFlowPublicHTTPDoer == nil {
				return missingEgress("tickflow authenticated/public HTTP endpoints")
			}
		case "pytdx":
			if deps.PytdxDialContext == nil { return missingEgress("pytdx TCP") }
		case "xueqiu":
			if deps.XueqiuHTTPDoer == nil { return missingEgress("xueqiu HTTP") }
		default:
			return fmt.Errorf("controlled egress validation does not recognize provider %q", name)
		}
	}
	if cfg.MXEnabled && deps.MiaoXiangHTTPDoer == nil {
		return missingEgress("miao-xiang HTTP")
	}
	return nil
}

func missingEgress(name string) error {
	return fmt.Errorf("controlled egress dependency missing: %s", name)
}

