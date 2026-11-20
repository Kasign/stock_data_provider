package service

import (
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
