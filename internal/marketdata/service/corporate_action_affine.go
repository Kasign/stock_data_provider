package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

type corporateActionAffineTransform struct {
	PriceScale     float64
	PriceOffsetCNY float64
}

type corporateActionAffineDateTerms struct {
	ShareRatio float64
	CashOffset float64
}

func corporateActionHasTerms(value model.CorporateAction) bool {
	return value.CashDividendCNY != nil || value.CashDividendBeforeTaxCNY != nil ||
		value.BonusShareRatio != nil || value.TransferShareRatio != nil ||
		value.RightsIssueRatio != nil || value.RightsIssuePriceCNY != nil ||
		value.CashConsiderationCNY != nil || value.ShareConsiderationRatio != nil
}

// composeCorporateActionAffineTransform builds price = scale*raw_price + offset.
// Events on the same ex-date are combined before composing dates in descending order.
func composeCorporateActionAffineTransform(
	events []model.CorporateAction,
	policy model.CorporateActionAdjustmentPolicy,
	holderScope model.CorporateActionHolderScope,
) (corporateActionAffineTransform, error) {
	if policy != model.CorporateActionAdjustmentPolicyExchangeReference &&
		policy != model.CorporateActionAdjustmentPolicyTradableHolderTotalReturn &&
		policy != model.CorporateActionAdjustmentPolicyProviderCompatibility {
		return corporateActionAffineTransform{}, fmt.Errorf("unsupported corporate-action adjustment policy %q", policy)
	}
	if policy == model.CorporateActionAdjustmentPolicyTradableHolderTotalReturn &&
		holderScope != model.CorporateActionHolderScopeAllShareholders &&
		holderScope != model.CorporateActionHolderScopeTradableShareholders &&
		holderScope != model.CorporateActionHolderScopeNonTradableShareholders {
		return corporateActionAffineTransform{}, fmt.Errorf("total-return adjustment requires an explicit supported holder scope")
	}

	byDate := make(map[string]*corporateActionAffineDateTerms)
	for _, event := range events {
		if !corporateActionHasTerms(event) {
			continue
		}
		exDate := strings.TrimSpace(event.ExDate)
		parsedDate, err := time.Parse("2006-01-02", exDate)
		if err != nil || parsedDate.Format("2006-01-02") != exDate {
			return corporateActionAffineTransform{}, fmt.Errorf("corporate action %s has invalid ex_date %q", event.ProviderEventID, event.ExDate)
		}

		includeOrdinary, includeConsideration := false, false
		switch policy {
		case model.CorporateActionAdjustmentPolicyExchangeReference:
			if event.ReferencePriceIncluded == nil {
				return corporateActionAffineTransform{}, fmt.Errorf("corporate action %s has unknown exchange-reference treatment", event.ProviderEventID)
			}
			if !*event.ReferencePriceIncluded {
				continue
			}
			if event.CashConsiderationCNY != nil || event.ShareConsiderationRatio != nil {
				return corporateActionAffineTransform{}, fmt.Errorf("corporate action %s mixes exchange-reference terms with shareholder consideration; split the event", event.ProviderEventID)
			}
			includeOrdinary = true
		case model.CorporateActionAdjustmentPolicyTradableHolderTotalReturn:
			if event.HolderScope == "" || event.HolderScope == model.CorporateActionHolderScopeUnknown {
				return corporateActionAffineTransform{}, fmt.Errorf("corporate action %s has unknown holder scope", event.ProviderEventID)
			}
			if event.HolderScope != model.CorporateActionHolderScopeAllShareholders && event.HolderScope != holderScope {
				continue
			}
			includeOrdinary, includeConsideration = true, true
		case model.CorporateActionAdjustmentPolicyProviderCompatibility:
			includeOrdinary, includeConsideration = true, true
		}

		cashDividend := 0.0
		if includeOrdinary {
			switch {
			case event.CashDividendBeforeTaxCNY != nil:
				cashDividend = *event.CashDividendBeforeTaxCNY
			case event.CashDividendCNY != nil && policy == model.CorporateActionAdjustmentPolicyProviderCompatibility:
				cashDividend = *event.CashDividendCNY
			case event.CashDividendCNY != nil:
				return corporateActionAffineTransform{}, fmt.Errorf("corporate action %s lacks pre-tax cash dividend required for %s", event.ProviderEventID, policy)
			}
		}

		bonus, transfer, rights := 0.0, 0.0, 0.0
		if includeOrdinary {
			if event.BonusShareRatio != nil {
				bonus = *event.BonusShareRatio
			}
			if event.TransferShareRatio != nil {
				transfer = *event.TransferShareRatio
			}
			if event.RightsIssueRatio != nil {
				rights = *event.RightsIssueRatio
			}
		}
		rightsPrice := 0.0
		if rights > 0 {
			if event.RightsIssuePriceCNY == nil {
				return corporateActionAffineTransform{}, fmt.Errorf("corporate action %s has rights ratio without rights issue price", event.ProviderEventID)
			}
			rightsPrice = *event.RightsIssuePriceCNY
		}

		considerationCash, considerationShares := 0.0, 0.0
		if includeConsideration {
			if event.CashConsiderationCNY != nil {
				considerationCash = *event.CashConsiderationCNY
			}
			if event.ShareConsiderationRatio != nil {
				considerationShares = *event.ShareConsiderationRatio
			}
		}
		shareRatio := bonus + transfer + rights + considerationShares
		cashOffset := cashDividend + considerationCash - rights*rightsPrice
		if math.IsNaN(shareRatio) || math.IsInf(shareRatio, 0) || math.IsNaN(cashOffset) || math.IsInf(cashOffset, 0) {
			return corporateActionAffineTransform{}, fmt.Errorf("corporate action %s has non-finite affine terms", event.ProviderEventID)
		}
		terms := byDate[exDate]
		if terms == nil {
			terms = &corporateActionAffineDateTerms{}
			byDate[exDate] = terms
		}
		terms.ShareRatio += shareRatio
		terms.CashOffset += cashOffset
	}

	dates := make([]string, 0, len(byDate))
	for date := range byDate {
		dates = append(dates, date)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dates)))

	transform := corporateActionAffineTransform{PriceScale: 1}
	for _, date := range dates {
		terms := byDate[date]
		multiplier := 1 + terms.ShareRatio
		if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) {
			return corporateActionAffineTransform{}, fmt.Errorf("corporate action date %s produces invalid share multiplier", date)
		}
		transform.PriceScale = multiplier * transform.PriceScale
		transform.PriceOffsetCNY = multiplier*transform.PriceOffsetCNY + terms.CashOffset
		if math.IsNaN(transform.PriceScale) || math.IsInf(transform.PriceScale, 0) ||
			math.IsNaN(transform.PriceOffsetCNY) || math.IsInf(transform.PriceOffsetCNY, 0) {
			return corporateActionAffineTransform{}, fmt.Errorf("corporate action date %s overflows affine transform", date)
		}
	}
	return transform, nil
}

// buildCorporateActionAdjustmentFactors creates shared factors for one exact raw series.
// It delegates all price math to composeCorporateActionAffineTransform.
func buildCorporateActionAdjustmentFactors(
	raw []model.DailyBar,
	events []model.CorporateAction,
	policy model.CorporateActionAdjustmentPolicy,
	holderScope model.CorporateActionHolderScope,
	coverageComplete bool,
	coverageFetchedAt time.Time,
	coverageAvailableAt *time.Time,
) ([]model.AdjustmentFactor, error) {
	if len(raw) == 0 || coverageFetchedAt.IsZero() {
		return nil, fmt.Errorf("raw bars and coverage fetched_at are required")
	}
	if coverageAvailableAt != nil && (coverageAvailableAt.IsZero() || coverageAvailableAt.After(coverageFetchedAt)) {
		return nil, fmt.Errorf("corporate-action coverage available_at is invalid")
	}
	symbol := strings.TrimSpace(raw[0].Symbol)
	rawSource := strings.TrimSpace(raw[0].Source)
	rawVersion := strings.TrimSpace(raw[0].SourceVersion)
	if symbol == "" || rawSource == "" || !hasIdentifiableSourceVersion(rawVersion) {
		return nil, fmt.Errorf("raw bars must identify symbol, source and source_version")
	}
	lastDate := ""
	for i, bar := range raw {
		if strings.TrimSpace(bar.Symbol) != symbol || strings.TrimSpace(bar.Source) != rawSource ||
			strings.TrimSpace(bar.SourceVersion) != rawVersion || bar.AdjustmentBasis != model.AdjustmentNone ||
			!validServiceDateRange(bar.TradeDate, bar.TradeDate) || (lastDate != "" && bar.TradeDate <= lastDate) {
			return nil, fmt.Errorf("raw bar %d does not belong to one ordered unadjusted source/version series", i)
		}
		lastDate = bar.TradeDate
	}

	evidenceEvents := make([]model.CorporateAction, 0, len(events))
	calculationEvents := make([]model.CorporateAction, 0, len(events))
	seenIdentity := make(map[string]struct{}, len(events))
	for _, event := range events {
		if event.Symbol != symbol {
			return nil, fmt.Errorf("corporate action %s belongs to %s, want %s", event.ProviderEventID, event.Symbol, symbol)
		}
		if event.NormalizedStatus == "proposal" || !corporateActionHasTerms(event) {
			continue
		}
		if event.ExDate == "" || !validServiceDateRange(event.ExDate, event.ExDate) {
			return nil, fmt.Errorf("corporate action %s has missing or invalid ex-date", event.ProviderEventID)
		}
		if event.ExDate > lastDate {
			continue
		}
		if event.ProviderEventID == "" || strings.TrimSpace(event.Source) == "" ||
			!hasIdentifiableSourceVersion(event.SourceVersion) || event.FetchedAt.IsZero() ||
			event.FetchedAt.After(coverageFetchedAt) {
			return nil, fmt.Errorf("corporate action on %s has incomplete source/fetch provenance", event.ExDate)
		}
		if event.AvailableAt != nil && (event.AvailableAt.IsZero() || event.AvailableAt.After(event.FetchedAt)) {
			return nil, fmt.Errorf("corporate action %s has invalid available_at", event.ProviderEventID)
		}
		identity := event.Source + "|" + event.SourceVersion + "|" + event.ProviderEventID
		if _, exists := seenIdentity[identity]; exists {
			return nil, fmt.Errorf("duplicate corporate-action event identity %s", identity)
		}
		seenIdentity[identity] = struct{}{}
		if policy != model.CorporateActionAdjustmentPolicyProviderCompatibility {
			if event.NormalizedStatus != "implemented" {
				return nil, fmt.Errorf("corporate action %s has unverified implementation status %q", identity, event.NormalizedStatus)
			}
			included, includeErr := corporateActionIncludedByPolicy(event, policy, holderScope)
			if includeErr != nil {
				return nil, fmt.Errorf("corporate action %s: %w", identity, includeErr)
			}
			if included && !event.TermsComplete {
				return nil, fmt.Errorf("corporate action %s has incomplete terms for policy %s", identity, policy)
			}
		}
		evidenceEvents = append(evidenceEvents, event)
		calculationEvents = append(calculationEvents, event)
	}
	calculationEvents, err := deduplicateCorporateActionEvents(calculationEvents)
	if err != nil {
		return nil, err
	}

	out := make([]model.AdjustmentFactor, 0, len(raw))
	for _, bar := range raw {
		asOfEvents := make([]model.CorporateAction, 0, len(calculationEvents))
		asOfEvidence := make([]model.CorporateAction, 0, len(evidenceEvents))
		for _, event := range calculationEvents {
			if event.ExDate <= bar.TradeDate {
				asOfEvents = append(asOfEvents, event)
			}
		}
		for _, event := range evidenceEvents {
			if event.ExDate <= bar.TradeDate {
				asOfEvidence = append(asOfEvidence, event)
			}
		}
		transform, err := composeCorporateActionAffineTransform(asOfEvents, policy, holderScope)
		if err != nil {
			return nil, fmt.Errorf("derive %s affine factor for %s: %w", bar.TradeDate, policy, err)
		}

		quality := model.QualityValid
		reasons := []string{}
		if !coverageComplete {
			quality = model.QualityDegraded
			appendReasonOnce(&reasons, "corporate_action_history_coverage_unverified")
		}
		availableAt := cloneCorporateActionAffineTime(coverageAvailableAt)
		sources := make([]string, 0, len(asOfEvidence))
		hashes := make([]string, 0, len(asOfEvidence))
		for _, event := range asOfEvidence {
			sources = append(sources, event.Source+"@"+event.SourceVersion)
			hash, err := corporateActionAffineEvidenceFingerprint(event)
			if err != nil {
				return nil, err
			}
			hashes = append(hashes, hash)
			quality = combineCorporateActionAffineQuality(quality, event.QualityStatus)
			for _, reason := range event.QualityReasons {
				appendReasonOnce(&reasons, "corporate_action:"+reason)
			}
			if policy == model.CorporateActionAdjustmentPolicyProviderCompatibility && !event.TermsComplete {
				quality = model.QualityDegraded
				appendReasonOnce(&reasons, "provider_compatibility_event_terms_completeness_unverified")
			}
			if event.AvailableAt == nil {
				availableAt = nil
				quality = model.QualityDegraded
				appendReasonOnce(&reasons, "corporate_action_available_at_unknown")
			} else if availableAt != nil && event.AvailableAt.After(*availableAt) {
				value := event.AvailableAt.UTC()
				availableAt = &value
			}
		}
		if availableAt == nil {
			if quality == model.QualityValid {
				quality = model.QualityDegraded
			}
			appendReasonOnce(&reasons, "corporate_action_factor_available_at_unknown")
		}
		sort.Strings(hashes)
		out = append(out, model.AdjustmentFactor{
			Symbol: symbol, TradeDate: bar.TradeDate, Factor: transform.PriceScale,
			PriceOffsetCNY: corporateActionAffineFloatPointer(transform.PriceOffsetCNY),
			Basis:                  model.AdjustmentFactorShared,
			Source:                 rawSource,
			SourceVersion:          rawVersion,
			Derivation:             "corporate_action_" + string(policy) + "_affine_v1",
			EvidenceSources:        uniqueSortedCorporateActionStrings(sources),
			EvidenceRevisionHashes: hashes, AvailableAt: availableAt,
			FetchedAt: coverageFetchedAt.UTC(), QualityStatus: quality, QualityReasons: reasons,
		})
	}
	return out, nil
}

func corporateActionIncludedByPolicy(event model.CorporateAction, policy model.CorporateActionAdjustmentPolicy,
	holderScope model.CorporateActionHolderScope) (bool, error) {
	switch policy {
	case model.CorporateActionAdjustmentPolicyExchangeReference:
		if event.ReferencePriceIncluded == nil {
			return false, fmt.Errorf("exchange-reference treatment is unknown")
		}
		return *event.ReferencePriceIncluded, nil
	case model.CorporateActionAdjustmentPolicyTradableHolderTotalReturn:
		if event.HolderScope == "" || event.HolderScope == model.CorporateActionHolderScopeUnknown {
			return false, fmt.Errorf("holder scope is unknown")
		}
		return event.HolderScope == model.CorporateActionHolderScopeAllShareholders || event.HolderScope == holderScope, nil
	case model.CorporateActionAdjustmentPolicyProviderCompatibility:
		return true, nil
	default:
		return false, fmt.Errorf("unsupported policy %q", policy)
	}
}

func deduplicateCorporateActionEvents(events []model.CorporateAction) ([]model.CorporateAction, error) {
	out := make([]model.CorporateAction, 0, len(events))
	byKey := make(map[string]int, len(events))
	for _, event := range events {
		eventType := strings.ToLower(strings.TrimSpace(event.EventType))
		if eventType == "" {
			return nil, fmt.Errorf("corporate action %s has empty event_type", event.ProviderEventID)
		}
		key := event.ExDate + "|" + eventType
		if index, exists := byKey[key]; exists {
			if !sameCorporateActionEconomicTerms(out[index], event) {
				return nil, fmt.Errorf("conflicting provider terms for same corporate-action type/date %s", key)
			}
			continue
		}
		byKey[key] = len(out)
		out = append(out, event)
	}
	return out, nil
}

func sameCorporateActionEconomicTerms(left, right model.CorporateAction) bool {
	return sameCorporateActionOptionalFloat(left.CashDividendCNY, right.CashDividendCNY) &&
		sameCorporateActionOptionalFloat(left.CashDividendBeforeTaxCNY, right.CashDividendBeforeTaxCNY) &&
		sameCorporateActionOptionalFloat(left.BonusShareRatio, right.BonusShareRatio) &&
		sameCorporateActionOptionalFloat(left.TransferShareRatio, right.TransferShareRatio) &&
		sameCorporateActionOptionalFloat(left.RightsIssueRatio, right.RightsIssueRatio) &&
		sameCorporateActionOptionalFloat(left.RightsIssuePriceCNY, right.RightsIssuePriceCNY) &&
		sameCorporateActionOptionalFloat(left.CashConsiderationCNY, right.CashConsiderationCNY) &&
		sameCorporateActionOptionalFloat(left.ShareConsiderationRatio, right.ShareConsiderationRatio) &&
		left.HolderScope == right.HolderScope &&
		sameCorporateActionOptionalBool(left.ReferencePriceIncluded, right.ReferencePriceIncluded)
}

func sameCorporateActionOptionalFloat(left, right *float64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return math.Abs(*left-*right) <= 1e-12
}

func sameCorporateActionOptionalBool(left, right *bool) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func corporateActionAffineEvidenceFingerprint(event model.CorporateAction) (string, error) {
	event.FetchedAt = time.Time{}
	event.SystemObservedAt = nil
	payload, err := json.Marshal(event)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}

func corporateActionAffineFloatPointer(value float64) *float64 { return &value }

func cloneCorporateActionAffineTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}

func uniqueSortedCorporateActionStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			set[value] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for value := range set {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func combineCorporateActionAffineQuality(left, right model.QualityStatus) model.QualityStatus {
	if left == model.QualityDegraded || right == model.QualityDegraded {
		return model.QualityDegraded
	}
	if left != model.QualityValid || right != model.QualityValid {
		return model.QualityUnknown
	}
	return model.QualityValid
}
