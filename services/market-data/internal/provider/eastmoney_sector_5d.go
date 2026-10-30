package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// fetchSectorRankings5D calculates five-trading-session return from each board's
// own daily K-line. It never uses a truncated current-day ranking as the candidate universe.
func (p *Eastmoney) fetchSectorRankings5D(ctx context.Context, category, direction string, limit int) (*model.SectorRankingSnapshot, error) {
	filter := map[string]string{"industry": "m:90+t:2+f:!50", "concept": "m:90+t:3+f:!50"}[category]
	if filter == "" {
		return nil, fmt.Errorf("unsupported sector category %q", category)
	}
	const pageSize = 100
	type board struct {
		code, name           string
		change, amount       *float64
		amountUnit           string
		advancing, declining *int
		leader               string
		leaderChange         *float64
	}
	candidates := make([]board, 0, 256)
	reportedTotal := -1
	for page := 1; ; page++ {
		if page > 20 {
			return nil, errors.New("Eastmoney sector candidate universe exceeds the 2000-row safety bound")
		}
		params := url.Values{
			"pn": {strconv.Itoa(page)}, "pz": {strconv.Itoa(pageSize)}, "po": {"1"}, "np": {"1"},
			"ut": {p.ut}, "fltt": {"2"}, "invt": {"2"}, "fid": {"f3"}, "fs": {filter},
			"fields": {"f2,f3,f4,f6,f12,f14,f104,f105,f128,f136"},
			"_":      {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}
		var payload struct {
			RC   *int `json:"rc"`
			Data *struct {
				Total *int                         `json:"total"`
				Diff  []map[string]json.RawMessage `json:"diff"`
			} `json:"data"`
		}
		if err := p.getJSONWithRetry(ctx, p.sectorRankingURL, params, &payload, 8*time.Second); err != nil {
			return nil, fmt.Errorf("Eastmoney sector 5-day candidate page %d: %w", page, err)
		}
		if payload.RC != nil && *payload.RC != 0 {
			return nil, fmt.Errorf("Eastmoney sector 5-day candidate provider error rc=%d", *payload.RC)
		}
		if payload.Data == nil || payload.Data.Total == nil || *payload.Data.Total <= 0 {
			return nil, errors.New("Eastmoney sector 5-day candidate universe has no verifiable total")
		}
		if reportedTotal < 0 {
			reportedTotal = *payload.Data.Total
			if reportedTotal > 2000 {
				return nil, fmt.Errorf("Eastmoney sector candidate total %d exceeds safety bound", reportedTotal)
			}
		} else if reportedTotal != *payload.Data.Total {
			return nil, errors.New("Eastmoney sector candidate total changed during pagination")
		}
		expected := pageSize
		if remaining := reportedTotal - len(candidates); remaining < expected {
			expected = remaining
		}
		if expected <= 0 {
			break
		}
		if len(payload.Data.Diff) != expected {
			return nil, fmt.Errorf("Eastmoney sector candidate page %d returned %d rows, expected %d", page, len(payload.Data.Diff), expected)
		}
		for _, raw := range payload.Data.Diff {
			code, name, change := parseEastmoneyBoardString(raw, "f12"), parseEastmoneyBoardString(raw, "f14"), parseEastmoneyBoardNumber(raw, "f3")
			if code == "" || name == "" || change == nil {
				return nil, errors.New("Eastmoney sector candidate row is missing code, name or change percent")
			}
			amount := parseEastmoneyBoardNumber(raw, "f6")
			unit := "CNY"
			if amount == nil {
				unit = "unknown"
			}
			candidates = append(candidates, board{
				code: code, name: name, change: change, amount: amount, amountUnit: unit,
				advancing: parseEastmoneyBoardInteger(raw, "f104"), declining: parseEastmoneyBoardInteger(raw, "f105"),
				leader: parseEastmoneyBoardString(raw, "f128"), leaderChange: parseEastmoneyBoardNumber(raw, "f136"),
			})
		}
		if len(candidates) == reportedTotal {
			break
		}
	}
	if len(candidates) != reportedTotal {
		return nil, errors.New("Eastmoney sector candidate pagination is incomplete")
	}

	type result struct {
		item model.SectorRanking
		date string
		err  error
	}
	jobs := make(chan board)
	results := make(chan result, len(candidates))
	workerCount := 4
	if len(candidates) < workerCount {
		workerCount = len(candidates)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for candidate := range jobs {
				item, date, err := p.fetchSectorBoard5D(ctx, category, candidate)
				results <- result{item: item, date: date, err: err}
			}
		}()
	}
	go func() {
		for _, candidate := range candidates {
			jobs <- candidate
		}
		close(jobs)
		wg.Wait()
		close(results)
	}()
	items := make([]model.SectorRanking, 0, len(candidates))
	reportDate := ""
	failed := 0
	fetchedAt := time.Now().UTC()
	for result := range results {
		if result.err != nil {
			failed++
			continue
		}
		if reportDate == "" {
			reportDate = result.date
		}
		if result.date != reportDate {
			failed++
			continue
		}
		result.item.Rank = len(items) + 1
		result.item.FetchedAt = fetchedAt
		items = append(items, result.item)
	}
	if len(items) == 0 {
		return nil, errors.New("Eastmoney sector 5-day ranking has no complete historical observations")
	}
	rankableTotal := len(items)
	sort.Slice(items, func(i, j int) bool {
		left, right := *items[i].ChangePercent5D, *items[j].ChangePercent5D
		if left != right {
			if direction == "up" {
				return left > right
			}
			return left < right
		}
		return items[i].Code < items[j].Code
	})
	if len(items) > limit {
		items = items[:limit]
	}
	for i := range items {
		items[i].Rank = i + 1
	}
	reasons := []string{"provider_snapshot_timestamp_not_exposed", "five_day_change_derived_from_six_daily_closes"}
	total := reportedTotal
	if failed > 0 {
		reasons = append(reasons, "provider_historical_board_coverage_incomplete")
		total = rankableTotal
	}
	for i := range items {
		items[i].FetchedAt = fetchedAt
	}
	return &model.SectorRankingSnapshot{
		Category: category, Direction: direction, SortBy: "change_5d", Total: total, ProviderReportDate: reportDate,
		ProviderReturned: len(candidates), TotalIsLowerBound: failed > 0,
		Items: items, Source: p.Name(), SourceVersion: "eastmoney-sector-5d-kline-v1",
		FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded, QualityReasons: reasons,
	}, nil
}

func (p *Eastmoney) fetchSectorBoard5D(ctx context.Context, category string, candidate struct {
	code, name           string
	change, amount       *float64
	amountUnit           string
	advancing, declining *int
	leader               string
	leaderChange         *float64
}) (model.SectorRanking, string, error) {
	params := url.Values{
		"secid": {"90." + candidate.code}, "fields1": {"f1,f2,f3,f4,f5,f6"},
		"fields2": {"f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61"},
		"klt":     {"101"}, "fqt": {"0"}, "beg": {"0"}, "end": {"20500101"}, "lmt": {"6"},
		"ut": {p.ut}, "_": {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Code   string   `json:"code"`
			KLines []string `json:"klines"`
		} `json:"data"`
	}
	if err := p.getJSONWithRetry(ctx, p.klineURL, params, &payload, 8*time.Second); err != nil {
		return model.SectorRanking{}, "", err
	}
	if payload.RC != nil && *payload.RC != 0 || payload.Data == nil {
		return model.SectorRanking{}, "", errors.New("Eastmoney sector K-line response is unavailable")
	}
	if payload.Data.Code != "" && payload.Data.Code != candidate.code {
		return model.SectorRanking{}, "", errors.New("Eastmoney sector K-line code mismatch")
	}
	if len(payload.Data.KLines) < 6 {
		return model.SectorRanking{}, "", errors.New("Eastmoney sector K-line has fewer than six daily closes")
	}
	type closeRow struct {
		date  string
		close float64
	}
	rows := make([]closeRow, 0, len(payload.Data.KLines))
	seen := map[string]bool{}
	for _, line := range payload.Data.KLines {
		parts := strings.Split(line, ",")
		if len(parts) < 3 {
			return model.SectorRanking{}, "", errors.New("Eastmoney sector K-line row is incomplete")
		}
		date := strings.TrimSpace(parts[0])
		if _, err := time.Parse("2006-01-02", date); err != nil || seen[date] {
			return model.SectorRanking{}, "", errors.New("Eastmoney sector K-line contains invalid or duplicate dates")
		}
		seen[date] = true
		closeValue, err := strconv.ParseFloat(parts[2], 64)
		if err != nil || math.IsNaN(closeValue) || math.IsInf(closeValue, 0) || closeValue <= 0 {
			return model.SectorRanking{}, "", errors.New("Eastmoney sector K-line contains an invalid close")
		}
		rows = append(rows, closeRow{date: date, close: closeValue})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].date < rows[j].date })
	if len(rows) > 6 {
		rows = rows[len(rows)-6:]
	}
	base, latest := rows[0].close, rows[len(rows)-1].close
	value := (latest/base - 1) * 100
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return model.SectorRanking{}, "", errors.New("Eastmoney sector 5-day return is non-finite")
	}
	item := model.SectorRanking{
		Code: candidate.code, Name: candidate.name, Category: category,
		ChangePercent: candidate.change, ChangePercent5D: &value,
		Amount: candidate.amount, AmountUnit: candidate.amountUnit,
		AdvancingCount: candidate.advancing, DecliningCount: candidate.declining,
		LeadingStock: candidate.leader, LeaderChangePercent: candidate.leaderChange,
		Source: p.Name(), SourceVersion: "eastmoney-sector-5d-kline-v1",
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed", "five_day_change_derived_from_six_daily_closes"},
	}
	return item, rows[len(rows)-1].date, nil
}
