package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/validation"
)

const maxProviderPayloadBytes int64 = 8 << 20

func readBoundedBody(r io.Reader, maxBytes int64) ([]byte, error) {
	if maxBytes <= 0 {
		return nil, errors.New("payload size limit must be positive")
	}
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read provider payload: %w", err)
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("provider payload exceeds %d byte limit", maxBytes)
	}
	return body, nil
}

func decodeSingleJSON(body []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("provider payload contains multiple JSON values")
		}
		return fmt.Errorf("provider payload has trailing non-JSON content: %w", err)
	}
	return nil
}

// normalizeProviderDailyBars 保留旧调用签名，兼容不需要请求级诊断的调用方。
func normalizeProviderDailyBars(symbol, startDate, endDate string, limit int, bars []model.DailyBar, rejectedRows int) ([]model.DailyBar, error) {
	return normalizeProviderDailyBarsWithReport(context.Background(), symbol, startDate, endDate, limit, bars, rejectedRows)
}

// normalizeProviderDailyBarsWithReport 在去重、过滤和排序前记录原始输入行诊断。
func normalizeProviderDailyBarsWithReport(ctx context.Context, symbol, startDate, endDate string, limit int, bars []model.DailyBar, rejectedRows int) ([]model.DailyBar, error) {
	if startDate != "" {
		if _, err := time.Parse("2006-01-02", startDate); err != nil {
			return nil, fmt.Errorf("invalid start date: %w", err)
		}
	}
	if endDate != "" {
		if _, err := time.Parse("2006-01-02", endDate); err != nil {
			return nil, fmt.Errorf("invalid end date: %w", err)
		}
	}
	if startDate != "" && endDate != "" && startDate > endDate {
		return nil, errors.New("start date must not be after end date")
	}

	normalized, warnings, report := normalize.ValidateAndSortBarsWithReport(symbol, bars, startDate, endDate, limit)
	hasSourceContractIssue := rejectedRows > 0
	seenDates := make(map[string]int, len(bars))
	for rowIndex, bar := range bars {
		if _, err := time.Parse("2006-01-02", bar.TradeDate); err != nil {
			hasSourceContractIssue = true
			continue
		}
		if firstIndex, exists := seenDates[bar.TradeDate]; exists {
			hasSourceContractIssue = true
			row := rowIndex
			report.Add(validation.FieldIssue{
				Stage: validation.StageNormalize, Field: "trade_date", ProviderField: "trade_date",
				Code: "duplicate_trade_date", Expected: "one daily bar per trade date",
				ActualType: "string", ActualPreview: bar.TradeDate,
				Severity: validation.SeverityWarning,
				Message:  fmt.Sprintf("duplicate daily bar; first occurrence is row %d", firstIndex),
				RowIndex: &row, Symbol: bar.Symbol, TradeDate: bar.TradeDate,
			})
		} else {
			seenDates[bar.TradeDate] = rowIndex
		}
	}
	if rejectedRows > 0 {
		report.Add(validation.FieldIssue{
			Stage: validation.StageParse, Field: "row", ProviderField: "daily_bar_row",
			Code: "rows_rejected_during_parse", Expected: "provider rows matching the daily bar schema",
			ActualType: "count", ActualPreview: strconv.Itoa(rejectedRows),
			Severity: validation.SeverityWarning,
			Message:  "provider rows were rejected during parsing; row-level details are unavailable",
		})
	}
	if hasSourceContractIssue || normalize.HasQualityDegradingBarWarnings(warnings) {
		for i := range normalized {
			if normalized[i].QualityStatus == model.QualityValid || normalized[i].QualityStatus == model.QualityUnknown || normalized[i].QualityStatus == "" {
				normalized[i].QualityStatus = model.QualityDegraded
			}
			normalized[i].QualityReasons = appendProviderQualityReason(normalized[i].QualityReasons, "provider_response_contract_warning")
			if rejectedRows > 0 {
				normalized[i].QualityReasons = appendProviderQualityReason(normalized[i].QualityReasons, "provider_rows_rejected_during_parse")
			}
		}
	}
	validation.AddContextReport(ctx, report)
	if len(normalized) == 0 {
		return nil, errors.New("provider returned no valid daily bars after normalization")
	}
	return normalized, nil
}

func appendProviderQualityReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func optionalVolumeUnit(value *int64, unit string) string {
	if value == nil {
		return "unknown"
	}
	return unit
}

// validProviderSnapshotTime rejects empty/zero timestamps and timestamps more than five minutes in the future.
// Older timestamps remain valid here; freshness policy is handled by the quote cache/service layer.
func validProviderSnapshotTime(value *time.Time, reference time.Time) *time.Time {
	if value == nil || value.IsZero() || value.After(reference.Add(5*time.Minute)) {
		return nil
	}
	return value
}

// addProviderParseIssue 将 Adapter 解码边界的问题追加到当前请求报告。
// 只接受字段值摘要，不接受完整响应、URL、Header 或认证信息。
func addProviderParseIssue(ctx context.Context, rowIndex int, field, providerField, code, expected, actualType, preview, message string) {
	preview = strings.ToValidUTF8(preview, "�")
	preview = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, preview)
	if utf8.RuneCountInString(preview) > 160 {
		runes := []rune(preview)
		preview = string(runes[:160])
	}
	var row *int
	if rowIndex >= 0 {
		value := rowIndex
		row = &value
	}
	report := validation.NewReport(validation.DefaultMaxIssues)
	report.Add(validation.FieldIssue{
		Stage: validation.StageParse, Field: field, ProviderField: providerField,
		Code: code, Expected: expected, ActualType: actualType, ActualPreview: preview,
		Severity: validation.SeverityWarning, Message: message, RowIndex: row,
	})
	validation.AddContextReport(ctx, report)
}

func providerJSONValueType(raw json.RawMessage) string {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return "missing"
	}
	if !json.Valid(value) {
		return "invalid"
	}
	switch value[0] {
	case 'n':
		return "null"
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	default:
		return "number"
	}
}

func providerJSONPreview(raw json.RawMessage) string {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || providerJSONValueType(raw) == "object" || providerJSONValueType(raw) == "array" {
		return ""
	}
	if providerJSONValueType(raw) == "string" {
		var decoded string
		if json.Unmarshal(value, &decoded) == nil {
			return decoded
		}
	}
	return string(value)
}
