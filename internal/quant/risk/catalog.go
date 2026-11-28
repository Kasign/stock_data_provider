package risk

import (
	"errors"
	"fmt"
)

// 风险规则目录（DEC-PLAT-002-059 后续项，执行清单 1.4 前置③）。
//
// 目录解决的是"规则从哪里来"：量化引擎不得发明规则，只能登记有明确依据的规则，
// 并把缺依据的规则位显式列出。发布规则集只能引用"已发布"条目，引用未登记、
// 未发布或仅有占位主题的 code 一律拒绝加载——这样"缺规则"表现为可观测的失败，
// 而不是悄悄少跑一条门禁。

var (
	ErrInvalidRuleCatalog   = errors.New("invalid risk rule catalog")
	ErrUnpublishedRuleUsage = errors.New("risk rule set references unpublished rule")
)

// RuleCatalogVersion 是规则目录自身的版本。它不进入 RiskDecision：
// 决策记录的是规则集版本（RuleSet.Version），目录版本用于治理与审计追溯。
const RuleCatalogVersion = "qnt-risk-catalog/1.0.0"

// InputQualityGateCode 是"输入质量不可用"硬门禁的规则编码。
// 依据执行清单 1.5 的封闭适用性映射：任一输入不可用时不得放行。
// 当前仅登记、未发布——Risk 评估链路尚未接入输入质量事实。
const InputQualityGateCode = "input_quality_not_applicable"

// RuleEntryStatus 表示一条规则在目录中的可用状态。
type RuleEntryStatus string

const (
	// RuleEntryPublished 表示规则有依据、已实现、可进入发布规则集。
	RuleEntryPublished RuleEntryStatus = "published"
	// RuleEntryRegistered 表示规则有依据、已实现，但尚未被任何发布规则集采用
	// （例如 Risk 评估链路还没接入对应输入）。
	RuleEntryRegistered RuleEntryStatus = "registered_not_published"
)

// RuleEntry 是目录中的一条规则及其依据。
type RuleEntry struct {
	Definition RuleDefinition
	Status     RuleEntryStatus
	// Source 说明该规则的依据来源（契约条款、文档章节或代码位置）。
	// 它必须可追溯；"业务惯例""经验值"不构成依据。
	Source string
}

// PendingRule 是尚未登记为规则的占位项：只有主题与缺失原因，没有严重级别。
// 它存在的意义是让"缺规则"可见，并且在有人误用时让加载失败。
type PendingRule struct {
	// Topic 是稳定的主题标识，例如 concentration_limit。
	Topic string
	// Code 是预留的规则编码；它不在 Entries 中，因此任何规则集引用它都会加载失败。
	Code   string
	Reason string
}

// PublishedRuleSet 是一个已发布的规则集：版本 + 有序规则 code 清单 + 采用的目录版本。
type PublishedRuleSet struct {
	Version        string
	CatalogVersion string
	RuleCodes      []string
}

// RuleCatalog 是版本化规则目录。
type RuleCatalog struct {
	Version   string
	Entries   []RuleEntry
	Pending   []PendingRule
	Published []PublishedRuleSet
}

// Validate 校验目录自身：身份、唯一性、依据可追溯、block 必须 required、
// 发布集合只能引用已发布条目，且每个发布集合必须至少包含一条 block 规则。
func (catalog RuleCatalog) Validate() error {
	if !canonicalIdentity(catalog.Version) || len(catalog.Entries) == 0 {
		return fmt.Errorf("%w: version or entries missing", ErrInvalidRuleCatalog)
	}
	entries := make(map[string]RuleEntry, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		if !canonicalIdentity(entry.Definition.Code) || !validSeverity(entry.Definition.Severity) ||
			!canonicalIdentity(entry.Source) {
			return fmt.Errorf("%w: entry %q has invalid code/severity/source", ErrInvalidRuleCatalog, entry.Definition.Code)
		}
		if _, exists := entries[entry.Definition.Code]; exists {
			return fmt.Errorf("%w: duplicate entry %q", ErrInvalidRuleCatalog, entry.Definition.Code)
		}
		if entry.Definition.Severity == SeverityBlock && !entry.Definition.Required {
			return fmt.Errorf("%w: block rule %q must be required", ErrInvalidRuleCatalog, entry.Definition.Code)
		}
		switch entry.Status {
		case RuleEntryPublished, RuleEntryRegistered:
		default:
			return fmt.Errorf("%w: entry %q has unknown status %q", ErrInvalidRuleCatalog, entry.Definition.Code, entry.Status)
		}
		entries[entry.Definition.Code] = entry
	}
	pendingCodes := make(map[string]struct{}, len(catalog.Pending))
	for _, pending := range catalog.Pending {
		if !canonicalIdentity(pending.Topic) || !canonicalIdentity(pending.Code) || !canonicalIdentity(pending.Reason) {
			return fmt.Errorf("%w: pending rule topic/code/reason must be non-blank", ErrInvalidRuleCatalog)
		}
		if _, exists := entries[pending.Code]; exists {
			return fmt.Errorf("%w: pending code %q already registered", ErrInvalidRuleCatalog, pending.Code)
		}
		if _, exists := pendingCodes[pending.Code]; exists {
			return fmt.Errorf("%w: duplicate pending code %q", ErrInvalidRuleCatalog, pending.Code)
		}
		pendingCodes[pending.Code] = struct{}{}
	}

	seenVersions := make(map[string]struct{}, len(catalog.Published))
	for _, published := range catalog.Published {
		if !canonicalIdentity(published.Version) || len(published.RuleCodes) == 0 {
			return fmt.Errorf("%w: published rule set version or rules missing", ErrInvalidRuleCatalog)
		}
		if _, exists := seenVersions[published.Version]; exists {
			return fmt.Errorf("%w: duplicate published version %q", ErrInvalidRuleCatalog, published.Version)
		}
		seenVersions[published.Version] = struct{}{}
		// 发布集合必须与目录同源：引用其它目录版本的规则集无法证明依据仍有效。
		if published.CatalogVersion != catalog.Version {
			return fmt.Errorf("%w: rule set %q references catalog %q, want %q",
				ErrInvalidRuleCatalog, published.Version, published.CatalogVersion, catalog.Version)
		}
		hasBlock := false
		seenCodes := make(map[string]struct{}, len(published.RuleCodes))
		for _, code := range published.RuleCodes {
			entry, exists := entries[code]
			if !exists {
				return fmt.Errorf("%w: rule set %q references unknown rule %q",
					ErrInvalidRuleCatalog, published.Version, code)
			}
			if entry.Status != RuleEntryPublished {
				return fmt.Errorf("%w: rule set %q references rule %q with status %q",
					ErrInvalidRuleCatalog, published.Version, code, entry.Status)
			}
			if _, exists := seenCodes[code]; exists {
				return fmt.Errorf("%w: rule set %q repeats rule %q", ErrInvalidRuleCatalog, published.Version, code)
			}
			seenCodes[code] = struct{}{}
			if entry.Definition.Severity == SeverityBlock {
				hasBlock = true
			}
		}
		if !hasBlock {
			return fmt.Errorf("%w: rule set %q has no block rule", ErrInvalidRuleCatalog, published.Version)
		}
	}
	return nil
}

// Load 按发布版本加载规则集。目录必须先通过 Validate；引用未被发布的规则
// 会返回 ErrUnpublishedRuleUsage，而不是加载一个少规则但仍可放行的集合。
func (catalog RuleCatalog) Load(version string) (RuleSet, error) {
	if err := catalog.Validate(); err != nil {
		return RuleSet{}, err
	}
	var published *PublishedRuleSet
	for index := range catalog.Published {
		if catalog.Published[index].Version == version {
			published = &catalog.Published[index]
			break
		}
	}
	if published == nil {
		return RuleSet{}, fmt.Errorf("%w: no published rule set %q", ErrInvalidRuleCatalog, version)
	}
	entries := make(map[string]RuleEntry, len(catalog.Entries))
	for _, entry := range catalog.Entries {
		entries[entry.Definition.Code] = entry
	}

	rules := make([]RuleDefinition, 0, len(published.RuleCodes))
	for _, code := range published.RuleCodes {
		entry, exists := entries[code]
		if !exists {
			return RuleSet{}, fmt.Errorf("%w: rule set %q references unknown rule %q",
				ErrUnpublishedRuleUsage, version, code)
		}
		if entry.Status != RuleEntryPublished {
			return RuleSet{}, fmt.Errorf("%w: rule set %q references rule %q with status %q",
				ErrUnpublishedRuleUsage, version, code, entry.Status)
		}
		rules = append(rules, entry.Definition)
	}
	ruleSet := RuleSet{Version: published.Version, Rules: rules}
	if err := ruleSet.Validate(); err != nil {
		return RuleSet{}, err
	}
	return ruleSet, nil
}

// PendingTopics 返回目录中尚未登记的规则主题，供诊断与文档生成使用。
// 返回副本，调用方修改不会影响目录。
func (catalog RuleCatalog) PendingTopics() []string {
	topics := make([]string, 0, len(catalog.Pending))
	for _, pending := range catalog.Pending {
		topics = append(topics, pending.Topic)
	}
	return topics
}

// DefaultRuleCatalog 返回当前生效的规则目录快照。
//
// 只登记有代码级或契约级依据的规则：
//   - tradability_unverified：可交易性事实缺失时 fail-closed（doc/quant/09）；
//   - input_quality_not_applicable：依据 1.5 的封闭适用性映射（quality_applicability.go），
//     已登记但未发布——Risk 评估链路尚未接入输入质量事实。
//
// 集中度、流动性、波动率、行业暴露等规则一律列为 Pending：它们需要权威阈值，
// 量化引擎不得自行猜测。
func DefaultRuleCatalog() RuleCatalog {
	return RuleCatalog{
		Version: RuleCatalogVersion,
		Entries: []RuleEntry{
			{
				Definition: RuleDefinition{Code: TradabilityGateCode, Severity: SeverityBlock, Required: true},
				Status:     RuleEntryPublished,
				Source:     "doc/quant/09-risk-tradability硬门禁切片与前置.md 第 1、4 节；可交易性事实缺失时 fail-closed",
			},
			{
				Definition: RuleDefinition{Code: InputQualityGateCode, Severity: SeverityBlock, Required: true},
				Status:     RuleEntryRegistered,
				Source:     "internal/quant/application/quality_applicability.go 封闭适用性映射（执行清单 1.5）",
			},
		},
		Pending: []PendingRule{
			{Topic: "concentration_limit", Code: "concentration_limit_unverified", Reason: "缺权威集中度阈值与组合口径，须业务规则来源"},
			{Topic: "liquidity_limit", Code: "liquidity_limit_unverified", Reason: "缺可成交量/换手率事实与权威阈值"},
			{Topic: "volatility_limit", Code: "volatility_limit_unverified", Reason: "缺权威波动率上限与观察窗口口径"},
			{Topic: "industry_exposure_limit", Code: "industry_exposure_limit_unverified", Reason: "缺行业分类映射与权威暴露上限"},
		},
		Published: []PublishedRuleSet{
			{
				Version:        TradabilityRuleSetVersion,
				CatalogVersion: RuleCatalogVersion,
				RuleCodes:      []string{TradabilityGateCode},
			},
		},
	}
}
