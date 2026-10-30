package service

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

var (
	probeHTTPStatusPattern = regexp.MustCompile(`(?i)\bHTTP(?:\s+status)?\s*[:=]?\s*(\d{3})\b`)
	probeURLQueryPattern   = regexp.MustCompile(`(?i)(https?://[^\s"'?]+)\?[^\s"' ]*`)
	probeSecretPattern     = regexp.MustCompile(`(?i)(token|api[_-]?key|apikey|cookie|authorization)(["'=:\s]+)([^&\s,;"']+)`)
)

type providerProbeFunc func(context.Context) (model.QualityStatus, string, []string, error)
type providerProbeJob struct {
	provider   string
	capability string
	run        providerProbeFunc
}

// ProbeProviderCapabilities 主动探测已启用 Provider 的核心能力。
// 每个探测直接调用对应 Provider，不经过业务请求的熔断统计；单项超时和并发均受限。
func (s *DataService) ProbeProviderCapabilities(ctx context.Context) (*model.ProviderProbeRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	startedAt := time.Now().UTC()
	run := &model.ProviderProbeRun{RunID: newFetchID(), StartedAt: startedAt, Items: make([]model.ProviderProbeItem, 0)}
	jobs := make([]providerProbeJob, 0)
	add := func(name, capability string, fn providerProbeFunc) {
		jobs = append(jobs, providerProbeJob{provider: name, capability: capability, run: fn})
	}
	const quoteSymbol = "600519.SH"
	const barsSymbol = "600036.SH"
	for _, p := range s.quoteProviders {
		p := p
		add(p.Name(), "quote", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			q, err := p.FetchQuote(probeCtx, quoteSymbol)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if q == nil {
				return model.QualityUnknown, "", nil, errors.New("empty quote response")
			}
			if q.Symbol == "" {
				q.Symbol = quoteSymbol
			}
			if q.Source == "" {
				q.Source = p.Name()
			}
			if err := normalize.ValidateQuote(q); err != nil {
				return model.QualityInvalid, q.SourceVersion, nil, err
			}
			if q.SourceVersion == "" {
				return model.QualityInvalid, "", nil, errors.New("quote source version missing")
			}
			return q.QualityStatus, q.SourceVersion, append([]string(nil), q.QualityReasons...), nil
		})
	}
	for _, p := range s.barProviders {
		p := p
		add(p.Name(), "daily_bars", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			bars, err := p.FetchDailyBars(probeCtx, barsSymbol, "", "", 5, model.AdjustmentNone)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			validated, warnings := normalize.ValidateAndSortBars(barsSymbol, bars, "", "", 5)
			if len(validated) == 0 {
				return model.QualityInvalid, "", warnings, errors.New("daily-bars probe returned no valid bars")
			}
			quality := model.QualityValid
			version := ""
			reasons := append([]string(nil), warnings...)
			for _, bar := range validated {
				if version == "" {
					version = bar.SourceVersion
				}
				if bar.QualityStatus != model.QualityValid {
					quality = model.QualityDegraded
				}
				for _, reason := range bar.QualityReasons {
					reasons = appendReason(reasons, reason)
				}
			}
			if len(warnings) > 0 {
				quality = model.QualityDegraded
			}
			if version == "" {
				return model.QualityInvalid, "", reasons, errors.New("daily-bars source version missing")
			}
			return quality, version, reasons, nil
		})
	}
	for _, p := range s.stockBasicProviders {
		p := p
		add(p.Name(), "stock_basic", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			item, err := p.FetchStockBasic(probeCtx, barsSymbol)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateStockBasic(barsSymbol, item); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return item.QualityStatus, item.SourceVersion, append([]string(nil), item.QualityReasons...), nil
		})
	}
	for _, p := range s.stockSearchProviders {
		p := p
		add(p.Name(), "stock_search", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			snapshot, err := p.SearchStocks(probeCtx, "600519", 1)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateStockSearchSnapshot("600519", 1, snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, append([]string(nil), snapshot.QualityReasons...), nil
		})
	}
	for _, p := range s.indexQuoteProviders {
		p := p
		add(p.Name(), "index_quotes", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			snapshot, err := p.FetchIndexQuotes(probeCtx)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateIndexQuoteSnapshot(snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, append([]string(nil), snapshot.QualityReasons...), nil
		})
	}
	for _, p := range s.marketBreadthProviders {
		p := p
		add(p.Name(), "market_breadth", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			snapshot, err := p.FetchMarketBreadth(probeCtx)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateMarketBreadth(snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, append([]string(nil), snapshot.QualityReasons...), nil
		})
	}
	for _, p := range s.sectorRankingProviders {
		p := p
		add(p.Name(), "sector_rankings", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			snapshot, err := p.FetchSectorRankings(probeCtx, "industry", 5)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateSectorRankingSnapshot("industry", 5, snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, append([]string(nil), snapshot.QualityReasons...), nil
		})
	}
	for _, p := range s.marketCapitalFlowProviders {
		p := p
		add(p.Name(), "market_capital_flow", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			loc := marketLocation()
			end := time.Now().In(loc)
			start := end.AddDate(0, 0, -14)
			snapshot, err := p.FetchMarketCapitalFlow(probeCtx, start.Format("2006-01-02"), end.Format("2006-01-02"))
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateMarketCapitalFlowSnapshot(start.Format("2006-01-02"), end.Format("2006-01-02"), snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, []string{"market_capital_flow_history_probe"}, nil
		})
	}
	run.Total = len(jobs)
	results := make([]model.ProviderProbeItem, len(jobs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(index int, job providerProbeJob) {
			defer wg.Done()
			item := model.ProviderProbeItem{Provider: job.provider, Capability: job.capability, Status: "failed", QualityStatus: model.QualityUnknown}
			checkedAt := time.Now().UTC()
			probeCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
			defer cancel()
			select {
			case sem <- struct{}{}:
			case <-probeCtx.Done():
				setProbeFailure(&item, probeCtx.Err())
				item.ElapsedMS = time.Since(checkedAt).Milliseconds()
				item.CheckedAt = time.Now().UTC()
				results[index] = item
				return
			}
			quality, version, reasons, err := job.run(probeCtx)
			<-sem
			item.ElapsedMS = time.Since(checkedAt).Milliseconds()
			item.CheckedAt = time.Now().UTC()
			item.SourceVersion = version
			item.QualityReasons = reasons
			if err != nil {
				setProbeFailure(&item, err)
			} else {
				item.QualityStatus = quality
				if quality == model.QualityValid {
					item.Status = "returned"
				} else {
					item.Status = "returned_degraded"
				}
			}
			results[index] = item
		}(i, job)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		return results[i].Capability < results[j].Capability
	})
	run.Items = results
	for _, item := range results {
		switch item.Status {
		case "returned":
			run.Returned++
		case "returned_degraded":
			run.Degraded++
		default:
			run.Failed++
		}
	}
	run.Status = "success"
	if run.Degraded > 0 {
		run.Status = "degraded"
	}
	if run.Failed > 0 {
		run.Status = "failed"
	}
	run.FinishedAt = time.Now().UTC()
	if len(jobs) == 0 {
		run.Status = "no_capabilities_configured"
	}
	if s.store != nil {
		run.Persisted = true
		if err := s.store.SaveProviderProbeRun(ctx, run); err != nil {
			run.Persisted = false
			return run, fmt.Errorf("persist provider-probe run: %w", err)
		}
	}
	return run, nil
}

func classifyProbeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "429"), strings.Contains(message, "too many requests"), strings.Contains(message, "rate limit"):
		return "rate_limited"
	case strings.Contains(message, "401"), strings.Contains(message, "unauthorized"), strings.Contains(message, "invalid token"):
		return "authentication_failed"
	case strings.Contains(message, "403"), strings.Contains(message, "forbidden"), strings.Contains(message, "permission denied"):
		return "permission_denied"
	}
	if match := probeHTTPStatusPattern.FindStringSubmatch(err.Error()); len(match) == 2 {
		return "http_" + match[1]
	}
	if strings.Contains(message, "contract") || strings.Contains(message, "invalid") || strings.Contains(message, "missing") || strings.Contains(message, "unexpected") {
		return "contract_invalid"
	}
	if strings.Contains(message, "no such host") || strings.Contains(message, "dial tcp") ||
		strings.Contains(message, "connection refused") || strings.Contains(message, "connection reset") ||
		strings.Contains(message, "eof") || strings.Contains(message, "network is unreachable") {
		return "network_error"
	}
	return "provider_error"
}

func probeHTTPStatus(err error) int {
	if err == nil {
		return 0
	}
	match := probeHTTPStatusPattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 0
	}
	status, _ := strconv.Atoi(match[1])
	return status
}

func sanitizeProbeError(err error) string {
	if err == nil {
		return ""
	}
	message := probeURLQueryPattern.ReplaceAllString(err.Error(), `$1?[query redacted]`)
	message = probeSecretPattern.ReplaceAllString(message, `$1$2[redacted]`)
	message = strings.TrimSpace(message)
	if len(message) > 400 {
		message = message[:400] + "…"
	}
	return message
}

func probeErrorStage(code string) string {
	switch {
	case code == "timeout":
		return "timeout"
	case code == "cancelled":
		return "cancellation"
	case code == "rate_limited":
		return "rate_limit"
	case code == "authentication_failed":
		return "provider_auth"
	case code == "permission_denied":
		return "provider_permission"
	case code == "contract_invalid":
		return "contract"
	case code == "network_error":
		return "connect"
	case strings.HasPrefix(code, "http_"):
		return "http"
	default:
		return "provider"
	}
}

func setProbeFailure(item *model.ProviderProbeItem, err error) {
	if item == nil || err == nil {
		return
	}
	item.ErrorCode = classifyProbeError(err)
	item.ErrorStage = probeErrorStage(item.ErrorCode)
	item.ErrorMessage = sanitizeProbeError(err)
	item.HTTPStatus = probeHTTPStatus(err)
}

// ProviderProbeHistory returns persisted active-probe runs.
func (s *DataService) ProviderProbeHistory(ctx context.Context, limit int) (*model.ProviderProbeHistory, error) {
	if s.store == nil {
		return nil, errors.New("provider-probe history storage is not enabled")
	}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("provider-probe history limit must be between 1 and 100")
	}
	items, err := s.store.ListProviderProbeRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	return &model.ProviderProbeHistory{Items: items, Total: len(items)}, nil
}
