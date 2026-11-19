package provider

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func dailyBarExactValuesFromStrings(open, high, low, close, rawVolume, rawAmount, rawAmountUnit, amountUnit string) *model.DailyBarExactValues {
	openValue, err := canonicalDailyBarDecimal(open)
	if err != nil {
		return nil
	}
	highValue, err := canonicalDailyBarDecimal(high)
	if err != nil {
		return nil
	}
	lowValue, err := canonicalDailyBarDecimal(low)
	if err != nil {
		return nil
	}
	closeValue, err := canonicalDailyBarDecimal(close)
	if err != nil {
		return nil
	}
	volumeValue, err := canonicalDailyBarDecimal(rawVolume)
	if err != nil {
		return nil
	}

	result := &model.DailyBarExactValues{
		Open: openValue, High: highValue, Low: lowValue, Close: closeValue, RawVolume: volumeValue,
		OpenEvidence: dailyBarDecimalEvidenceFromString(open), HighEvidence: dailyBarDecimalEvidenceFromString(high),
		LowEvidence: dailyBarDecimalEvidenceFromString(low), CloseEvidence: dailyBarDecimalEvidenceFromString(close),
		RawVolumeEvidence: dailyBarDecimalEvidenceFromString(rawVolume),
	}
	if strings.TrimSpace(rawAmount) == "" {
		return result
	}
	amountValue, err := canonicalDailyBarDecimal(rawAmount)
	if err != nil {
		return nil
	}
	result.RawAmount = &amountValue
	result.RawAmountEvidence = dailyBarDecimalEvidenceFromString(rawAmount)
	if strings.EqualFold(strings.TrimSpace(rawAmountUnit), "cny") && strings.EqualFold(strings.TrimSpace(amountUnit), "cny") {
		result.AmountCNY = &amountValue
		result.AmountCNYEvidence = cloneDailyBarDecimalEvidence(result.RawAmountEvidence)
	}
	return result
}

func dailyBarExactValuesFromJSON(open, high, low, close, rawVolume, rawAmount json.RawMessage, rawAmountUnit, amountUnit string) *model.DailyBarExactValues {
	openValue, ok := dailyBarDecimalFromJSON(open)
	if !ok {
		return nil
	}
	highValue, ok := dailyBarDecimalFromJSON(high)
	if !ok {
		return nil
	}
	lowValue, ok := dailyBarDecimalFromJSON(low)
	if !ok {
		return nil
	}
	closeValue, ok := dailyBarDecimalFromJSON(close)
	if !ok {
		return nil
	}
	volumeValue, ok := dailyBarDecimalFromJSON(rawVolume)
	if !ok {
		return nil
	}
	amountValue, _ := dailyBarDecimalFromJSON(rawAmount)
	result := dailyBarExactValuesFromStrings(openValue, highValue, lowValue, closeValue, volumeValue, amountValue, rawAmountUnit, amountUnit)
	if result == nil {
		return nil
	}
	result.OpenEvidence = dailyBarDecimalEvidenceFromJSON(open)
	result.HighEvidence = dailyBarDecimalEvidenceFromJSON(high)
	result.LowEvidence = dailyBarDecimalEvidenceFromJSON(low)
	result.CloseEvidence = dailyBarDecimalEvidenceFromJSON(close)
	result.RawVolumeEvidence = dailyBarDecimalEvidenceFromJSON(rawVolume)
	result.RawAmountEvidence = dailyBarDecimalEvidenceFromJSON(rawAmount)
	if result.AmountCNY != nil {
		result.AmountCNYEvidence = cloneDailyBarDecimalEvidence(result.RawAmountEvidence)
	}
	return result
}

func dailyBarDecimalEvidenceFromString(raw string) *model.DailyBarDecimalEvidence {
	token := strings.TrimSpace(raw)
	if token == "" {
		return nil
	}
	if _, err := canonicalDailyBarDecimal(token); err != nil {
		return nil
	}
	mantissa := token
	if index := strings.IndexAny(mantissa, "eE"); index >= 0 {
		mantissa = mantissa[:index]
	}
	fractionDigits := 0
	if index := strings.IndexByte(mantissa, '.'); index >= 0 {
		fractionDigits = len(mantissa) - index - 1
	}
	return &model.DailyBarDecimalEvidence{
		EvidenceKind: model.DailyBarPrecisionEvidenceObservedToken,
		SourceToken: token,
		SourceFractionDigits: &fractionDigits,
	}
}

func dailyBarDecimalEvidenceFromJSON(raw json.RawMessage) *model.DailyBarDecimalEvidence {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return nil
	}
	if strings.HasPrefix(value, """) {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return nil
		}
		value = decoded
	}
	return dailyBarDecimalEvidenceFromString(value)
}

func cloneDailyBarDecimalEvidence(source *model.DailyBarDecimalEvidence) *model.DailyBarDecimalEvidence {
	if source == nil {
		return nil
	}
	result := *source
	if source.SourceFractionDigits != nil {
		value := *source.SourceFractionDigits
		result.SourceFractionDigits = &value
	}
	if source.ProtocolScale != nil {
		value := *source.ProtocolScale
		result.ProtocolScale = &value
	}
	return &result
}

func dailyBarDecimalFromJSON(raw json.RawMessage) (string, bool) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "", false
	}
	if strings.HasPrefix(value, "\"") {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return "", false
		}
		value = decoded
	}
	canonical, err := canonicalDailyBarDecimal(value)
	return canonical, err == nil
}

func canonicalDailyBarDecimal(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("decimal value is empty")
	}
	sign := ""
	if value[0] == '-' || value[0] == '+' {
		if value[0] == '-' {
			sign = "-"
		}
		value = value[1:]
	}
	exponent := 0
	if index := strings.IndexAny(value, "eE"); index >= 0 {
		parsed, err := strconv.Atoi(value[index+1:])
		if err != nil || parsed < -100 || parsed > 100 {
			return "", errors.New("decimal exponent is invalid or out of range")
		}
		exponent = parsed
		value = value[:index]
	}
	if strings.Count(value, ".") > 1 {
		return "", errors.New("decimal contains multiple points")
	}
	parts := strings.SplitN(value, ".", 2)
	integerPart := parts[0]
	fractionPart := ""
	if len(parts) == 2 {
		fractionPart = parts[1]
	}
	if integerPart == "" && fractionPart == "" {
		return "", errors.New("decimal contains no digits")
	}
	if integerPart == "" {
		integerPart = "0"
	}
	if !decimalDigitsOnly(integerPart) || !decimalDigitsOnly(fractionPart) {
		return "", errors.New("decimal contains invalid digits")
	}
	digits := integerPart + fractionPart
	decimalPosition := len(integerPart) + exponent
	var whole, fraction string
	switch {
	case decimalPosition <= 0:
		whole = "0"
		fraction = strings.Repeat("0", -decimalPosition) + digits
	case decimalPosition >= len(digits):
		whole = digits + strings.Repeat("0", decimalPosition-len(digits))
	default:
		whole = digits[:decimalPosition]
		fraction = digits[decimalPosition:]
	}
	whole = strings.TrimLeft(whole, "0")
	if whole == "" {
		whole = "0"
	}
	fraction = strings.TrimRight(fraction, "0")
	if whole == "0" && fraction == "" {
		return "0", nil
	}
	if fraction == "" {
		return sign + whole, nil
	}
	return sign + whole + "." + fraction, nil
}

func decimalDigitsOnly(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func scaledDecimalFromInt64(value int64, scale int) string {
	if scale < 0 || scale > 100 {
		return ""
	}
	if scale == 0 {
		return strconv.FormatInt(value, 10)
	}
	result, err := canonicalDailyBarDecimal(strconv.FormatInt(value, 10) + "e-" + strconv.Itoa(scale))
	if err != nil {
		return ""
	}
	return result
}

func dailyBarJSONValueAt(values []json.RawMessage, index int) json.RawMessage {
	if index < 0 || index >= len(values) {
		return nil
	}
	return values[index]
}
