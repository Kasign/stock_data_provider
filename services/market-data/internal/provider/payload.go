package provider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
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

// normalizeProviderDailyBars 统一整理公开数据源的日 K 返回，避免顺序、重复日期或越界记录直接流入分析主链路。
func normalizeProviderDailyBars(symbol, startDate, endDate string, limit int, bars []model.DailyBar, rejectedRows int) ([]model.DailyBar, error) {
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

	hasSourceContractIssue := rejectedRows > 0
	for _, bar := range bars {
		if _, err := time.Parse("2006-01-02", bar.TradeDate); err != nil {
			hasSourceContractIssue = true
			continue
		}
		// 上游返回请求范围外的日期，不能静默地混入本次查询结果。
		if (startDate != "" && bar.TradeDate < startDate) || (endDate != "" && bar.TradeDate > endDate) {
			hasSourceContractIssue = true
		}
	}
	normalized, warnings := normalize.ValidateAndSortBars(symbol, bars, startDate, endDate, limit)
	if len(normalized) == 0 {
		return nil, errors.New("provider returned no valid daily bars after normalization")
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
