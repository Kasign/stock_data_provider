package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/validation"
	"math"
	"strconv"
	"strings"
)

func parseProviderQuoteNumber(ctx context.Context, provider, providerField, field string, raw json.RawMessage, exists, required bool) (*float64, error) {
	s := strings.TrimSpace(string(raw))
	typ, value := "missing", ""
	switch {
	case !exists || s == "":
	case s == "null":
		typ = "null"
	case strings.HasPrefix(s, "{"):
		typ = "object"
	case strings.HasPrefix(s, "["):
		typ = "array"
	case s == "true" || s == "false":
		typ = "boolean"
	case strings.HasPrefix(s, "\""):
		typ = "string"
		_ = json.Unmarshal(raw, &value)
	default:
		typ = "number"
		value = s
	}
	preview := quoteIssuePreview(s)
	missing := typ == "missing" || typ == "null" || (typ == "string" && strings.TrimSpace(value) == "")
	severity := validation.SeverityError
	if !required {
		severity = validation.SeverityWarning
	}
	if missing {
		if !required {
			return nil, nil
		}
		code := "missing"
		if typ == "null" {
			code = "null_value"
		}
		addProviderQuoteIssue(ctx, validation.StageParse, field, providerField, code, "required finite number", typ, preview, provider+" quote required field is missing", severity)
		return nil, fmt.Errorf("%s quote field %s (%s) is missing", provider, field, providerField)
	}
	if typ != "number" && typ != "string" {
		addProviderQuoteIssue(ctx, validation.StageParse, field, providerField, "type_mismatch", "number or numeric string", typ, preview, provider+" quote field has unsupported JSON type", severity)
		if required {
			return nil, fmt.Errorf("%s quote field %s (%s) has type %s", provider, field, providerField, typ)
		}
		return nil, nil
	}
	n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		addProviderQuoteIssue(ctx, validation.StageParse, field, providerField, "invalid_number", "finite number", typ, preview, provider+" quote field is not finite", severity)
		if required {
			return nil, fmt.Errorf("%s quote field %s (%s) is not finite", provider, field, providerField)
		}
		return nil, nil
	}
	return &n, nil
}
func addProviderQuoteIssue(ctx context.Context, stage validation.Stage, field, providerField, code, expected, actualType, preview, message string, severity validation.Severity) {
	r := validation.NewReport(1)
	r.Add(validation.FieldIssue{Stage: stage, Field: field, ProviderField: providerField, Code: code, Expected: expected, ActualType: actualType, ActualPreview: preview, Severity: severity, Message: message})
	validation.AddContextReport(ctx, r)
}
func quoteIssuePreview(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return ' '
		}
		return r
	}, s)
	if len(s) > 160 {
		return s[:160]
	}
	return s
}
