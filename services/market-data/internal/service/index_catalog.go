package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

func (s *DataService) SetIndexCatalogProviders(providers []provider.IndexCatalogProvider) {
	s.indexCatalogProviders = append([]provider.IndexCatalogProvider(nil), providers...)
}

func (s *DataService) IndexCatalogProviders() []provider.IndexCatalogProvider {
	return append([]provider.IndexCatalogProvider(nil), s.indexCatalogProviders...)
}

func canonicalIndexSymbol(raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	parts := strings.Split(value, ".")
	if len(parts) != 2 || (parts[1] != "TI" && parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "CSI") || len(parts[0]) != 6 {
		return "", fmt.Errorf("index symbol must be six digits with .TI/.SH/.SZ/.CSI suffix: %q", raw)
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("index symbol must use six numeric digits: %q", raw)
		}
	}
	return value, nil
}

func validIndexCatalogTag(raw string) (string, error) {
	tag := strings.ToLower(strings.TrimSpace(raw))
	if tag == "" {
		tag = "cn_concept"
	}
	switch tag {
	case "cn_concept", "region", "tszs", "industry", "tushare":
		return tag, nil
	default:
		return "", fmt.Errorf("tag must be cn_concept, region, tszs, industry or tushare")
	}
}

func validSnapshotQuality(q model.QualityStatus) bool {
	return q == model.QualityValid || q == model.QualityDegraded || q == model.QualityUnknown
}

func validateIndexCatalogSnapshot(tag string, snapshot *model.IndexCatalogSnapshot) error {
	if snapshot == nil || snapshot.Tag != tag || snapshot.Total != len(snapshot.Items) {
		return errors.New("index catalog snapshot identity/count mismatch")
	}
	if snapshot.Source == "" || snapshot.FetchedAt.IsZero() || !validSnapshotQuality(snapshot.QualityStatus) {
		return errors.New("index catalog snapshot source, timestamp or quality invalid")
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		symbol, err := canonicalIndexSymbol(item.Symbol)
		if err != nil || symbol != item.Symbol || strings.TrimSpace(item.Name) == "" || item.Tag != tag {
			return errors.New("index catalog contains invalid symbol/name/tag")
		}
		if _, exists := seen[symbol]; exists {
			return fmt.Errorf("duplicate index catalog symbol %s", symbol)
		}
		seen[symbol] = struct{}{}
	}
	if len(snapshot.Items) == 0 && snapshot.QualityStatus != model.QualityUnknown {
		return errors.New("empty index catalog must be marked unknown")
	}
	return nil
}

func validateIndexConstituentsSnapshot(indexSymbol string, snapshot *model.IndexConstituentSnapshot) error {
	if snapshot == nil || snapshot.IndexSymbol != indexSymbol || snapshot.Total != len(snapshot.Items) {
		return errors.New("index constituent snapshot identity/count mismatch")
	}
	if snapshot.Source == "" || snapshot.FetchedAt.IsZero() || !validSnapshotQuality(snapshot.QualityStatus) {
		return errors.New("index constituent snapshot source, timestamp or quality invalid")
	}
	if snapshot.ProviderReportDate != "" {
		date, err := time.Parse("2006-01-02", snapshot.ProviderReportDate)
		if err != nil || date.Format("2006-01-02") != snapshot.ProviderReportDate {
			return errors.New("index constituent provider_report_date is invalid")
		}
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		symbol, err := normalize.NormalizeSymbol(item.Symbol)
		if err != nil || symbol != item.Symbol || strings.TrimSpace(item.Name) == "" || item.Code != strings.Split(symbol, ".")[0] {
			return errors.New("index constituent contains invalid symbol/code/name")
		}
		if item.WeightPercent != nil && (math.IsNaN(*item.WeightPercent) || math.IsInf(*item.WeightPercent, 0) || *item.WeightPercent < 0 || *item.WeightPercent > 100) {
			return errors.New("index constituent weight_percent must be between 0 and 100 when present")
		}
		if _, exists := seen[symbol]; exists {
			return fmt.Errorf("duplicate index constituent %s", symbol)
		}
		seen[symbol] = struct{}{}
	}
	if len(snapshot.Items) == 0 && snapshot.QualityStatus != model.QualityUnknown {
		return errors.New("empty index constituents must be marked unknown")
	}
	return nil
}

func validateIndexDailyBarsSnapshot(indexSymbol, startDate, endDate string, snapshot *model.IndexDailyBarsSnapshot) error {
	if snapshot == nil || snapshot.IndexSymbol != indexSymbol || snapshot.StartDate != startDate || snapshot.EndDate != endDate || snapshot.Total != len(snapshot.Items) {
		return errors.New("index daily-bars snapshot identity/range/count mismatch")
	}
	if snapshot.Source == "" || snapshot.FetchedAt.IsZero() || !validSnapshotQuality(snapshot.QualityStatus) {
		return errors.New("index daily-bars snapshot source, timestamp or quality invalid")
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	previousDate := ""
	for _, item := range snapshot.Items {
		if item.IndexSymbol != indexSymbol || item.TradeDate < startDate || item.TradeDate > endDate || item.Source != snapshot.Source || item.FetchedAt.IsZero() {
			return errors.New("index daily bar identity, date or source invalid")
		}
		if _, err := time.Parse("2006-01-02", item.TradeDate); err != nil {
			return errors.New("index daily bar has invalid trade date")
		}
		if _, duplicate := seen[item.TradeDate]; duplicate {
			return fmt.Errorf("duplicate index daily bar date %s", item.TradeDate)
		}
		seen[item.TradeDate] = struct{}{}
		if previousDate != "" && item.TradeDate < previousDate {
			return errors.New("index daily bars are not in ascending date order")
		}
		previousDate = item.TradeDate
		if math.IsNaN(item.Open) || math.IsInf(item.Open, 0) || math.IsNaN(item.High) || math.IsInf(item.High, 0) ||
			math.IsNaN(item.Low) || math.IsInf(item.Low, 0) || math.IsNaN(item.Close) || math.IsInf(item.Close, 0) ||
			item.Open <= 0 || item.High <= 0 || item.Low <= 0 || item.Close <= 0 ||
			item.Low > item.High || item.Open < item.Low || item.Open > item.High || item.Close < item.Low || item.Close > item.High {
			return fmt.Errorf("index daily bar %s has invalid OHLC values", item.TradeDate)
		}
		if item.Volume != nil && *item.Volume < 0 {
			return fmt.Errorf("index daily bar %s has negative volume", item.TradeDate)
		}
		if item.Amount != nil && (math.IsNaN(*item.Amount) || math.IsInf(*item.Amount, 0) || *item.Amount < 0) {
			return fmt.Errorf("index daily bar %s has invalid amount", item.TradeDate)
		}
		if !validSnapshotQuality(item.QualityStatus) || item.QualityStatus == model.QualityUnknown {
			return fmt.Errorf("index daily bar %s has unsupported quality", item.TradeDate)
		}
	}
	if len(snapshot.Items) == 0 && snapshot.QualityStatus != model.QualityUnknown {
		return errors.New("empty index daily-bars snapshot must be marked unknown")
	}
	return nil
}

func (s *DataService) IndexCatalog(ctx context.Context, rawTag string) (*model.IndexCatalogSnapshot, bool, error) {
	tag, err := validIndexCatalogTag(rawTag)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key := "md:v1:index-catalog:" + tag
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.IndexCatalogSnapshot
		if json.Unmarshal(data, &cached) == nil && validateIndexCatalogSnapshot(tag, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := 12 * time.Hour
			if cached.QualityStatus != model.QualityValid {
				ttl = time.Hour
			}
			if time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("index catalog cache read failed", "tag", tag, "error", cacheErr)
	}
	if len(s.indexCatalogProviders) == 0 {
		return nil, false, errors.New("no enabled index-catalog provider")
	}
	var failures []string
	for _, p := range s.indexCatalogProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchIndexCatalog(ctx, tag)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateIndexCatalogSnapshot(tag, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if payload, err := json.Marshal(snapshot); err == nil {
			ttl := 12 * time.Hour
			if snapshot.QualityStatus != model.QualityValid {
				ttl = time.Hour
			}
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("index catalog cache write failed", "tag", tag, "error", cacheErr)
			}
		} else {
			s.logger.Warn("index catalog cache encode failed", "tag", tag, "error", err)
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all index-catalog providers failed: %s", strings.Join(failures, "; "))
}

func (s *DataService) IndexConstituents(ctx context.Context, rawIndexSymbol string) (*model.IndexConstituentSnapshot, bool, error) {
	indexSymbol, err := canonicalIndexSymbol(rawIndexSymbol)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key := "md:v1:index-constituents:" + indexSymbol
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.IndexConstituentSnapshot
		if json.Unmarshal(data, &cached) == nil && validateIndexConstituentsSnapshot(indexSymbol, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := 30 * time.Minute
			if cached.QualityStatus != model.QualityValid {
				ttl = 5 * time.Minute
			}
			if time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("index constituents cache read failed", "symbol", indexSymbol, "error", cacheErr)
	}
	if len(s.indexCatalogProviders) == 0 {
		return nil, false, errors.New("no enabled index-catalog provider")
	}
	var failures []string
	for _, p := range s.indexCatalogProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchIndexConstituents(ctx, indexSymbol)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateIndexConstituentsSnapshot(indexSymbol, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if payload, err := json.Marshal(snapshot); err == nil {
			ttl := 30 * time.Minute
			if snapshot.QualityStatus != model.QualityValid {
				ttl = 5 * time.Minute
			}
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("index constituents cache write failed", "symbol", indexSymbol, "error", cacheErr)
			}
		} else {
			s.logger.Warn("index constituents cache encode failed", "symbol", indexSymbol, "error", err)
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all index-constituent providers failed: %s", strings.Join(failures, "; "))
}

func (s *DataService) IndexDailyBars(ctx context.Context, rawIndexSymbol, rawStartDate, rawEndDate string) (*model.IndexDailyBarsSnapshot, bool, error) {
	indexSymbol, err := canonicalIndexSymbol(rawIndexSymbol)
	if err != nil {
		return nil, false, err
	}
	startDate, endDate := strings.TrimSpace(rawStartDate), strings.TrimSpace(rawEndDate)
	start, startErr := time.Parse("2006-01-02", startDate)
	end, endErr := time.Parse("2006-01-02", endDate)
	if startErr != nil || start.Format("2006-01-02") != startDate || endErr != nil || end.Format("2006-01-02") != endDate {
		return nil, false, errors.New("start and end must use YYYY-MM-DD")
	}
	if start.After(end) {
		return nil, false, errors.New("start must not be after end")
	}
	if end.After(start.AddDate(10, 0, 0)) {
		return nil, false, errors.New("index daily-bars date range must not exceed 10 years")
	}
	if int(end.Sub(start)/(24*time.Hour))+1 > 3660 {
		return nil, false, errors.New("index daily-bars date range must not exceed 3660 calendar days")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	key := fmt.Sprintf("md:v1:index-daily-bars:%s:%s:%s", indexSymbol, startDate, endDate)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.IndexDailyBarsSnapshot
		if json.Unmarshal(data, &cached) == nil && validateIndexDailyBarsSnapshot(indexSymbol, startDate, endDate, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := 15 * time.Minute
			if cached.QualityStatus != model.QualityValid {
				ttl = 3 * time.Minute
			}
			if time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("index daily-bars cache read failed", "symbol", indexSymbol, "error", cacheErr)
	}
	if len(s.indexCatalogProviders) == 0 {
		return nil, false, errors.New("no enabled index-catalog provider")
	}
	var failures []string
	for _, p := range s.indexCatalogProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchIndexDailyBars(ctx, indexSymbol, startDate, endDate)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateIndexDailyBarsSnapshot(indexSymbol, startDate, endDate, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		sort.Slice(snapshot.Items, func(i, j int) bool { return snapshot.Items[i].TradeDate < snapshot.Items[j].TradeDate })
		if payload, err := json.Marshal(snapshot); err == nil {
			ttl := 15 * time.Minute
			if snapshot.QualityStatus != model.QualityValid {
				ttl = 3 * time.Minute
			}
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("index daily-bars cache write failed", "symbol", indexSymbol, "error", cacheErr)
			}
		} else {
			s.logger.Warn("index daily-bars cache encode failed", "symbol", indexSymbol, "error", err)
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all index daily-bars providers failed: %s", strings.Join(failures, "; "))
}
