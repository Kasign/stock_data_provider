package provider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const eastmoneyCapitalFlowVersion = "eastmoney-stock-capital-flow-v1"

type eastmoneyCapitalFlowObservation struct {
	date   string
	amount *float64
}

// FetchCapitalFlow 获取东方财富个股日主力净流入。金额按人民币元解释，源端未提供可验证的精确可用时间。
func (p *Eastmoney) FetchCapitalFlow(ctx context.Context, rawSymbol string) (*model.CapitalFlow, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Eastmoney capital flow only supports common A-share equities, got %q", rawSymbol)
	}
	secid, err := eastmoneySecID(symbol)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimSpace(p.capitalFlowURL)
	if endpoint == "" {
		endpoint = defaultEastmoneyCapitalFlowURL
	}
	params := url.Values{
		"secid":   {secid},
		"fields1": {"f1,f2,f3,f7"},
		"fields2": {"f51,f52"},
		"klt":     {"101"},
		"lmt":     {"20"},
		"end":     {"20500101"},
		"ut":      {p.ut},
		"_":       {strconv.FormatInt(time.Now().UnixMilli(), 10)},
	}
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Code   string   `json:"code"`
			KLines []string `json:"klines"`
		} `json:"data"`
	}
	if err := p.getJSONWithRetry(ctx, endpoint, params, &payload, 8*time.Second); err != nil {
		return nil, fmt.Errorf("Eastmoney capital-flow request: %w", err)
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney capital-flow provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil || len(payload.Data.KLines) == 0 {
		return nil, errors.New("Eastmoney capital-flow response has no daily observations")
	}
	if code := strings.TrimSpace(payload.Data.Code); code != "" && code != strings.Split(symbol, ".")[0] {
		return nil, fmt.Errorf("Eastmoney capital-flow symbol mismatch: requested %s, returned code %q", symbol, code)
	}

	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	observations := make([]eastmoneyCapitalFlowObservation, 0, len(payload.Data.KLines))
	seen := make(map[string]struct{}, len(payload.Data.KLines))
	for index, line := range payload.Data.KLines {
		fields := strings.Split(line, ",")
		if len(fields) < 2 {
			return nil, fmt.Errorf("Eastmoney capital-flow row %d has fewer than two fields", index)
		}
		date := strings.TrimSpace(fields[0])
		parsedDate, dateErr := time.Parse("2006-01-02", date)
		if dateErr != nil || parsedDate.Format("2006-01-02") != date {
			return nil, fmt.Errorf("Eastmoney capital-flow row %d has an invalid date", index)
		}
		if date > today {
			return nil, fmt.Errorf("Eastmoney capital-flow row date %s is in the future", date)
		}
		if _, exists := seen[date]; exists {
			return nil, fmt.Errorf("Eastmoney capital-flow response contains duplicate date %s", date)
		}
		seen[date] = struct{}{}

		rawAmount := strings.TrimSpace(fields[1])
		var amount *float64
		if rawAmount != "" && rawAmount != "-" && !strings.EqualFold(rawAmount, "null") {
			value, parseErr := strconv.ParseFloat(rawAmount, 64)
			if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("Eastmoney capital-flow row %s has an invalid main net-inflow amount", date)
			}
			amount = &value
		}
		observations = append(observations, eastmoneyCapitalFlowObservation{date: date, amount: amount})
	}
	sort.Slice(observations, func(i, j int) bool {
		return observations[i].date > observations[j].date
	})
	if len(observations) > 10 {
		observations = observations[:10]
	}
	if len(observations) == 0 {
		return nil, errors.New("Eastmoney capital-flow response contains no usable observations")
	}

	now := time.Now().UTC()
	out := &model.CapitalFlow{
		Symbol: symbol, ProviderDate: observations[0].date,
		SourceVersion: eastmoneyCapitalFlowVersion, Observations: len(observations),
		Source: p.Name(), FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed", "eastmoney_daily_main_net_inflow_series"},
	}
	out.MainNetInflow = observations[0].amount
	out.Inflow5D = sumEastmoneyCapitalFlowWindow(observations, 5)
	out.Inflow10D = sumEastmoneyCapitalFlowWindow(observations, 10)
	if out.MainNetInflow == nil {
		out.QualityReasons = append(out.QualityReasons, "latest_main_net_inflow_missing_or_invalid")
	}
	if out.Inflow5D == nil {
		out.QualityReasons = append(out.QualityReasons, "inflow_5d_requires_5_complete_observations")
	}
	if out.Inflow10D == nil {
		out.QualityReasons = append(out.QualityReasons, "inflow_10d_requires_10_complete_observations")
	}
	return out, nil
}

func sumEastmoneyCapitalFlowWindow(observations []eastmoneyCapitalFlowObservation, size int) *float64 {
	if len(observations) < size {
		return nil
	}
	sum := 0.0
	for _, item := range observations[:size] {
		if item.amount == nil {
			return nil
		}
		sum += *item.amount
		if math.IsNaN(sum) || math.IsInf(sum, 0) {
			return nil
		}
	}
	return &sum
}
