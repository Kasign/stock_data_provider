package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// FetchMarketCapitalFlow uses Tushare's moneyflow_mkt_dc market-wide daily dataset.
// It is deliberately a separate capability from per-stock capital flow.
func (p *Tushare) FetchMarketCapitalFlow(ctx context.Context, startDate, endDate string) (*model.MarketCapitalFlowSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("Tushare HTTP client is not configured")
	}
	if strings.TrimSpace(p.token) == "" {
		return nil, errors.New("Tushare token is not configured")
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil || start.Format("2006-01-02") != startDate {
		return nil, errors.New("Tushare market-capital-flow start date must use YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil || end.Format("2006-01-02") != endDate || end.Before(start) {
		return nil, errors.New("Tushare market-capital-flow end date must use YYYY-MM-DD and not precede start")
	}
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if end.Format("2006-01-02") > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("Tushare market-capital-flow end date must not be in the future")
	}
	if end.Sub(start) > 2000*24*time.Hour {
		return nil, errors.New("Tushare market-capital-flow range must not exceed 2000 calendar days")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := p.waitForRateLimit(ctx); err != nil {
		return nil, err
	}

	fields := "trade_date,close_sh,pct_change_sh,close_sz,pct_change_sz,net_amount,net_amount_rate,buy_elg_amount,buy_elg_amount_rate,buy_lg_amount,buy_lg_amount_rate,buy_md_amount,buy_md_amount_rate,buy_sm_amount,buy_sm_amount_rate"
	payload := map[string]any{
		"api_name": "moneyflow_mkt_dc",
		"token": p.token,
		"params": map[string]string{
			"start_date": start.Format("20060102"),
			"end_date": end.Format("20060102"),
		},
		"fields": fields,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode Tushare market-capital-flow request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create Tushare market-capital-flow request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MarketDataService/1.0")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Tushare market-capital-flow request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("Tushare market-capital-flow HTTP status %d", resp.StatusCode)
	}
	raw, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("read Tushare market-capital-flow response: %w", err)
	}
	var envelope struct {
		Code *int   `json:"code"`
		Msg  string `json:"msg"`
		Data *struct {
			Fields []string            `json:"fields"`
			Items  [][]json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode Tushare market-capital-flow response: %w", err)
	}
	if envelope.Code == nil {
		return nil, errors.New("Tushare market-capital-flow response is missing code")
	}
	if *envelope.Code != 0 {
		return nil, fmt.Errorf("Tushare market-capital-flow provider error code=%d", *envelope.Code)
	}
	if envelope.Data == nil {
		return nil, errors.New("Tushare market-capital-flow response is missing data")
	}
	indices := make(map[string]int, len(envelope.Data.Fields))
	for i, field := range envelope.Data.Fields {
		field = strings.TrimSpace(field)
		if field == "" {
			return nil, errors.New("Tushare market-capital-flow response contains an empty field name")
		}
		if _, exists := indices[field]; exists {
			return nil, fmt.Errorf("Tushare market-capital-flow response contains duplicate field %q", field)
		}
		indices[field] = i
	}
	if _, ok := indices["trade_date"]; !ok {
		return nil, errors.New("Tushare market-capital-flow response is missing trade_date")
	}
	if _, ok := indices["net_amount"]; !ok {
		return nil, errors.New("Tushare market-capital-flow response is missing net_amount")
	}
	if len(envelope.Data.Items) == 0 {
		return nil, errors.New("Tushare returned no market-capital-flow observations")
	}

	fetchedAt := time.Now().UTC()
	reasons := []string{"provider_snapshot_timestamp_not_exposed"}
	out := &model.MarketCapitalFlowSnapshot{
		StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
		Source: p.Name(), SourceVersion: "tushare-moneyflow-mkt-dc-v1",
		FetchedAt: fetchedAt, QualityStatus: model.QualityDegraded,
		Items: make([]model.MarketCapitalFlowItem, 0, len(envelope.Data.Items)),
	}
	seen := make(map[string]struct{}, len(envelope.Data.Items))
	read := func(row []json.RawMessage, rowIndex int, field string) (*float64, error) {
		index, ok := indices[field]
		if !ok {
			return nil, nil
		}
		if index >= len(row) {
			return nil, fmt.Errorf("Tushare market-capital-flow row %d is missing field %q", rowIndex, field)
		}
		rawValue := bytes.TrimSpace(row[index])
		if len(rawValue) == 0 || bytes.Equal(rawValue, []byte("null")) {
			return nil, nil
		}
		value, parseErr := tushareRawFloat(rawValue)
		if parseErr != nil {
			return nil, fmt.Errorf("Tushare market-capital-flow row %d has invalid %s", rowIndex, field)
		}
		return &value, nil
	}
	for rowIndex, row := range envelope.Data.Items {
		dateIndex := indices["trade_date"]
		if dateIndex >= len(row) {
			return nil, fmt.Errorf("Tushare market-capital-flow row %d is missing trade_date", rowIndex)
		}
		dateText, parseErr := tushareString(row[dateIndex])
		if parseErr != nil {
			return nil, fmt.Errorf("Tushare market-capital-flow row %d has invalid trade_date", rowIndex)
		}
		tradeDate, parseErr := time.Parse("20060102", dateText)
		if parseErr != nil {
			tradeDate, parseErr = time.Parse("2006-01-02", dateText)
		}
		if parseErr != nil {
			return nil, fmt.Errorf("Tushare market-capital-flow row %d has invalid trade_date %q", rowIndex, dateText)
		}
		date := tradeDate.Format("2006-01-02")
		if date < out.StartDate || date > out.EndDate {
			return nil, fmt.Errorf("Tushare market-capital-flow row date %s is outside requested range", date)
		}
		if _, exists := seen[date]; exists {
			return nil, fmt.Errorf("Tushare market-capital-flow response contains duplicate trade date %s", date)
		}
		seen[date] = struct{}{}
		item := model.MarketCapitalFlowItem{TradeDate: date}
		fields := []struct {
			name string
			dest **float64
		}{
			{"close_sh", &item.CloseSH}, {"pct_change_sh", &item.ChangePercentSH},
			{"close_sz", &item.CloseSZ}, {"pct_change_sz", &item.ChangePercentSZ},
			{"net_amount", &item.NetAmountCNY}, {"net_amount_rate", &item.NetAmountRatePercent},
			{"buy_elg_amount", &item.ExtraLargeNetInflowCNY}, {"buy_elg_amount_rate", &item.ExtraLargeNetInflowRatePct},
			{"buy_lg_amount", &item.LargeNetInflowCNY}, {"buy_lg_amount_rate", &item.LargeNetInflowRatePercent},
			{"buy_md_amount", &item.MediumNetInflowCNY}, {"buy_md_amount_rate", &item.MediumNetInflowRatePercent},
			{"buy_sm_amount", &item.SmallNetInflowCNY}, {"buy_sm_amount_rate", &item.SmallNetInflowRatePercent},
		}
		for _, field := range fields {
			value, valueErr := read(row, rowIndex, field.name)
			if valueErr != nil {
				return nil, valueErr
			}
			*field.dest = value
			if value == nil {
				reasons = appendMarketCapitalFlowReason(reasons, "market_flow_field_missing_or_null")
			}
		}
		out.Items = append(out.Items, item)
	}
	sort.Slice(out.Items, func(i, j int) bool { return out.Items[i].TradeDate < out.Items[j].TradeDate })
	out.Total = len(out.Items)
	out.QualityReasons = reasons
	return out, nil
}

func appendMarketCapitalFlowReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}
