package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

const corporateActionExchangeReferenceDerivation = "corporate_action_exchange_reference_affine_v1"

type corporateActionAffineTerm struct {
	exDate string
	scale  float64
	offset float64
	source string
	hash   string
	action model.CorporateAction
}

// deriveAdjustmentFactorsFromCorporateActions creates one shared affine factor per raw date.
// Factor source/version bind the result to the raw series; EvidenceSources identify event providers.
// coverageComplete must only be true when event history is verified from the listing date.
func deriveAdjustmentFactorsFromCorporateActions(raw []model.DailyBar, actions []model.CorporateAction,
	policy model.CorporateActionAdjustmentPolicy, coverageComplete bool,
	coverageFetchedAt time.Time, coverageAvailableAt *time.Time) ([]model.AdjustmentFactor, error) {
	if policy != model.CorporateActionAdjustmentPolicyExchangeReference {
		return nil, fmt.Errorf("unsupported corporate-action adjustment policy %q", policy)
	}
	if len(raw) == 0 || len(actions) == 0 || coverageFetchedAt.IsZero() {
		return nil, errors.New("raw bars, corporate actions and coverage fetched_at are required")
	}
	symbol, rawSource, rawVersion := strings.TrimSpace(raw[0].Symbol), strings.TrimSpace(raw[0].Source), strings.TrimSpace(raw[0].SourceVersion)
	if symbol == "" || rawSource == "" || !hasIdentifiableSourceVersion(rawVersion) {
		return nil, errors.New("raw bars must identify symbol, source and source_version")
	}
	if coverageAvailableAt != nil && (coverageAvailableAt.IsZero() || coverageAvailableAt.After(coverageFetchedAt)) {
		return nil, errors.New("coverage available_at must not be zero or after fetched_at")
	}
	previousDate := ""
	for i, bar := range raw {
		if strings.TrimSpace(bar.Symbol) != symbol || strings.TrimSpace(bar.Source) != rawSource ||
			strings.TrimSpace(bar.SourceVersion) != rawVersion || bar.AdjustmentBasis != model.AdjustmentNone {
			return nil, errors.New("rebuild requires one unadjusted raw series from a single provider/version")
		}
		if !validServiceDateRange(bar.TradeDate, bar.TradeDate) || (previousDate != "" && bar.TradeDate <= previousDate) {
			return nil, fmt.Errorf("raw bar %d has invalid or unsorted trade date %q", i, bar.TradeDate)
		}
		previousDate = bar.TradeDate
	}

	terms := make([]corporateActionAffineTerm, 0, len(actions))
	seen := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if strings.TrimSpace(action.Symbol) != symbol {
			return nil, fmt.Errorf("corporate action %s belongs to %s, want %s", action.ProviderEventID, action.Symbol, symbol)
		}
		if action.ExDate == "" && action.NormalizedStatus == "proposal" {
			continue
		}
		if action.ExDate == "" || !validServiceDateRange(action.ExDate, action.ExDate) {
			return nil, fmt.Errorf("corporate action %s has no valid ex-date", action.ProviderEventID)
		}
		if action.ExDate > raw[len(raw)-1].TradeDate {
			continue
		}
		if action.ProviderEventID == "" || strings.TrimSpace(action.Source) == "" || !hasIdentifiableSourceVersion(action.SourceVersion) {
			return nil, errors.New("corporate-action evidence must identify event, source and source_version")
		}
		identity := action.Source + "|" + action.SourceVersion + "|" + action.ProviderEventID
		if _, ok := seen[identity]; ok {
			return nil, fmt.Errorf("duplicate corporate-action evidence %s", identity)
		}
		seen[identity] = struct{}{}
		if action.FetchedAt.IsZero() || action.FetchedAt.After(coverageFetchedAt) {
			return nil, fmt.Errorf("corporate action %s was not included in the stated coverage fetch", identity)
		}
		if action.AvailableAt != nil && (action.AvailableAt.IsZero() || action.AvailableAt.After(action.FetchedAt)) {
			return nil, fmt.Errorf("corporate action %s has invalid available_at", identity)
		}
		if action.NormalizedStatus != "implemented" {
			return nil, fmt.Errorf("corporate action %s has unsupported status %q", identity, action.NormalizedStatus)
		}
		if action.ReferencePriceIncluded == nil {
			return nil, fmt.Errorf("corporate action %s has unknown reference-price eligibility", identity)
		}
		if !*action.ReferencePriceIncluded {
			continue
		}
		if action.HolderScope == model.CorporateActionHolderScopeUnknown ||
			action.HolderScope == model.CorporateActionHolderScopeNonTradableShareholders {
			return nil, fmt.Errorf("corporate action %s has incompatible holder scope %q", identity, action.HolderScope)
		}
		if !action.TermsComplete {
			return nil, fmt.Errorf("corporate action %s has incomplete economic terms", identity)
		}
		multiplier, offset, err := corporateActionExchangeReferenceTerm(action)
		if err != nil {
			return nil, fmt.Errorf("corporate action %s: %w", identity, err)
		}
		hash, err := corporateActionEvidenceFingerprint(action)
		if err != nil {
			return nil, err
		}
		terms = append(terms, corporateActionAffineTerm{exDate: action.ExDate, scale: multiplier, offset: offset,
			source: action.Source + "@" + action.SourceVersion, hash: hash, action: action})
	}
	if len(terms) == 0 {
		return nil, errors.New("no exchange-reference-eligible implemented corporate actions")
	}
	sort.Slice(terms, func(i, j int) bool {
		if terms[i].exDate != terms[j].exDate {
			return terms[i].exDate < terms[j].exDate
		}
		if terms[i].source != terms[j].source {
			return terms[i].source < terms[j].source
		}
		return terms[i].action.ProviderEventID < terms[j].action.ProviderEventID
	})
	aggregated := make([]corporateActionAffineTerm, 0, len(terms))
	for _, term := range terms {
		if len(aggregated) == 0 || aggregated[len(aggregated)-1].exDate != term.exDate {
			aggregated = append(aggregated, term)
			continue
		}
		last := &aggregated[len(aggregated)-1]
		last.scale += term.scale - 1
		last.offset += term.offset
		last.source += "," + term.source
		last.hash += "," + term.hash
		last.action.QualityStatus = combineCorporateActionQuality(last.action.QualityStatus, term.action.QualityStatus)
		last.action.QualityReasons = append(last.action.QualityReasons, term.action.QualityReasons...)
		if term.action.AvailableAt == nil {
			last.action.AvailableAt = nil
		} else if last.action.AvailableAt != nil && term.action.AvailableAt.After(*last.action.AvailableAt) {
			value := term.action.AvailableAt.UTC()
			last.action.AvailableAt = &value
		}
	}
	out := make([]model.AdjustmentFactor, 0, len(raw))
	scale, offset, nextTerm := 1.0, 0.0, 0
	for _, bar := range raw {
		for nextTerm < len(aggregated) && aggregated[nextTerm].exDate <= bar.TradeDate {
			previousScale := scale
			scale = previousScale * aggregated[nextTerm].scale
			offset += previousScale * aggregated[nextTerm].offset
			nextTerm++
		}
		quality := model.QualityValid
		reasons := []string{}
		if !coverageComplete {
			degradeQualityStatus(&quality)
			appendReasonOnce(&reasons, "corporate_action_history_coverage_unverified")
		}
		availableAt := cloneCorporateActionTime(coverageAvailableAt)
		hashes, sources := []string{}, []string{}
		for i := 0; i < nextTerm; i++ {
			term := aggregated[i]
			hashes = append(hashes, strings.Split(term.hash, ",")...)
			sources = append(sources, strings.Split(term.source, ",")...)
			quality = combineCorporateActionQuality(quality, term.action.QualityStatus)
			for _, reason := range term.action.QualityReasons {
				appendReasonOnce(&reasons, "corporate_action:"+reason)
			}
			if term.action.AvailableAt == nil {
				availableAt = nil
				degradeQualityStatus(&quality)
				appendReasonOnce(&reasons, "corporate_action_available_at_unknown")
			} else if availableAt != nil && term.action.AvailableAt.After(*availableAt) {
				value := term.action.AvailableAt.UTC()
				availableAt = &value
			}
		}
		if availableAt == nil {
			degradeQualityStatus(&quality)
			appendReasonOnce(&reasons, "corporate_action_factor_available_at_unknown")
		}
		sort.Strings(hashes)
		out = append(out, model.AdjustmentFactor{
			Symbol: symbol, TradeDate: bar.TradeDate, Factor: scale, PriceOffsetCNY: corporateActionFloatPointer(offset),
			Basis: model.AdjustmentFactorShared, Source: rawSource, SourceVersion: rawVersion,
			Derivation:      corporateActionExchangeReferenceDerivation,
			EvidenceSources: uniqueSortedStrings(sources), EvidenceRevisionHashes: hashes,
			AvailableAt: availableAt, FetchedAt: coverageFetchedAt.UTC(), QualityStatus: quality, QualityReasons: reasons,
		})
	}
	return out, nil
}

// P_pre = (1 + bonus + transfer + rights) * P_post + gross_cash - rights_price * rights.
func corporateActionExchangeReferenceTerm(action model.CorporateAction) (float64, float64, error) {
	zero := 0.0
	bonus, transfer, cash := action.BonusShareRatio, action.TransferShareRatio, action.CashDividendBeforeTaxCNY
	rights, rightsPrice := action.RightsIssueRatio, action.RightsIssuePriceCNY
	switch strings.ToLower(strings.TrimSpace(action.EventType)) {
	case "dividend", "standard_distribution":
		if bonus == nil || transfer == nil || cash == nil {
			return 0, 0, errors.New("distribution requires explicit bonus, transfer and gross cash terms")
		}
	case "rights_issue":
		if bonus == nil {
			bonus = &zero
		}
		if transfer == nil {
			transfer = &zero
		}
		if cash == nil {
			cash = &zero
		}
		if rights == nil || rightsPrice == nil {
			return 0, 0, errors.New("rights issue requires explicit ratio and subscription price")
		}
	case "share_reform_consideration":
		return 0, 0, errors.New("share-reform consideration cannot enter exchange-reference adjustment")
	default:
		return 0, 0, fmt.Errorf("unsupported event type %q", action.EventType)
	}
	if rights == nil {
		rights = &zero
	}
	if rightsPrice == nil {
		rightsPrice = &zero
	}
	for name, value := range map[string]*float64{"bonus_share_ratio": bonus, "transfer_share_ratio": transfer,
		"cash_dividend_before_tax": cash, "rights_issue_ratio": rights, "rights_issue_price": rightsPrice} {
		if value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 {
			return 0, 0, fmt.Errorf("%s is missing or invalid", name)
		}
	}
	multiplier := 1 + *bonus + *transfer + *rights
	offset := *cash - *rightsPrice**rights
	if multiplier <= 0 || math.IsNaN(multiplier) || math.IsInf(multiplier, 0) || math.IsNaN(offset) || math.IsInf(offset, 0) {
		return 0, 0, errors.New("invalid exchange-reference affine term")
	}
	return multiplier, offset, nil
}

func corporateActionEvidenceFingerprint(action model.CorporateAction) (string, error) {
	action.FetchedAt = time.Time{}
	action.SystemObservedAt = nil
	payload, err := json.Marshal(action)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]), nil
}
func corporateActionFloatPointer(value float64) *float64 { return &value }
func cloneCorporateActionTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := value.UTC()
	return &copy
}
func uniqueSortedStrings(values []string) []string {
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
func combineCorporateActionQuality(left, right model.QualityStatus) model.QualityStatus {
	if left == model.QualityDegraded || right == model.QualityDegraded {
		return model.QualityDegraded
	}
	if left == model.QualityUnknown || right == model.QualityUnknown {
		return model.QualityUnknown
	}
	return model.QualityValid
}
