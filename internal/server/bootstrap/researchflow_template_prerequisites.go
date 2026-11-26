package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

const (
	newsSearchParameterSchemaRef      = "doc/researchflow/schema/news-search-parameters-1.0.0.schema.json"
	newsSearchParameterSchemaVersion  = "1.0.0"
	newsSearchApplicabilityRuleRef    = "news.search_candidates.always"
	newsSearchApplicabilityVersion    = "1.0.0"
	newsFeedParameterSchemaRef        = "doc/researchflow/schema/news-feed-collection-parameters-1.0.0.schema.json"
	newsFeedParameterSchemaVersion    = "1.0.0"
	newsFeedApplicabilityRuleRef      = "news.feed_collection.configured_sources"
	newsFeedApplicabilityVersion      = "1.0.0"
	quantSMA20ParameterSchemaRef      = "doc/researchflow/schema/quant-sma20-parameters-1.0.0.schema.json"
	quantSMA20ParameterSchemaVersion  = "1.0.0"
	quantSMA20ApplicabilityRuleRef    = "quant.sma20.always"
	quantSMA20ApplicabilityVersion    = "1.0.0"
	quantFactorParameterSchemaRef     = "doc/researchflow/schema/quant-factor-parameters-1.0.0.schema.json"
	quantFactorParameterSchemaVersion = "1.0.0"
	quantFactorApplicabilityRuleRef   = "quant.factor.always"
	quantFactorApplicabilityVersion   = "1.0.0"
	quantTrendParameterSchemaRef      = "doc/researchflow/schema/quant-trend-parameters-1.0.0.schema.json"
	quantTrendParameterSchemaVersion  = "1.0.0"
	quantTrendApplicabilityRuleRef    = "quant.trend.always"
	quantTrendApplicabilityVersion    = "1.0.0"
)

var ErrUnsupportedNewsSearchApplicabilityRule = errors.New("unsupported News Search applicability rule")

const newsSearchParameterSchemaDocument = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:kasign:researchflow:news-search-parameters:1.0.0",
  "title": "News Search Parameters",
  "type": "object",
  "properties": {
    "query": {
      "type": "string",
      "minLength": 1,
      "pattern": "\\S"
    },
    "query_dimension": {
      "type": "string"
    },
    "limit": {
      "type": "integer",
      "minimum": 1,
      "maximum": 100
    }
  },
  "required": ["query", "limit"],
  "additionalProperties": false
}`

const newsFeedParameterSchemaDocument = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:kasign:researchflow:news-feed-collection-parameters:1.0.0",
  "title": "News Feed Collection Parameters",
  "type": "object",
  "properties": {},
  "additionalProperties": false
}`

type newsSearchParameterSchemaRegistry struct{}

func (newsSearchParameterSchemaRegistry) Get(ctx context.Context, ref string) (parameterschema.Definition, error) {
	if ctx == nil {
		return parameterschema.Definition{}, errors.New("News Search parameter schema context is required")
	}
	if err := ctx.Err(); err != nil {
		return parameterschema.Definition{}, err
	}
	if ref != newsSearchParameterSchemaRef {
		return parameterschema.Definition{}, fmt.Errorf("%w: %s", parameterschema.ErrSchemaNotFound, ref)
	}
	return parameterschema.Definition{
		Ref: newsSearchParameterSchemaRef, Version: newsSearchParameterSchemaVersion,
		Document: json.RawMessage(newsSearchParameterSchemaDocument),
	}, nil
}

func newNewsSearchParameterSchemaValidator() (*parameterschema.Validator, error) {
	return parameterschema.NewValidator(newsSearchParameterSchemaRegistry{})
}

type newsSearchApplicabilityEvaluator struct{}

func (newsSearchApplicabilityEvaluator) Evaluate(
	ctx context.Context,
	rule template.ApplicabilityRule,
	parameters json.RawMessage,
) (bool, string, error) {
	if ctx == nil {
		return false, "", errors.New("News Search applicability context is required")
	}
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	if rule.RuleRef != newsSearchApplicabilityRuleRef || rule.RuleVersion != newsSearchApplicabilityVersion {
		return false, "", fmt.Errorf("%w: %s@%s", ErrUnsupportedNewsSearchApplicabilityRule, rule.RuleRef, rule.RuleVersion)
	}
	// This rule is intentionally unconditional: the first production workflow
	// represented by this rule is a News Search workflow, so its News discovery
	// stage is always part of the DAG. Conditional business variants require a
	// new immutable rule ref/version rather than changing this rule in place.
	var object map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &object); err != nil || object == nil {
		return false, "", errors.New("News Search applicability requires validated object parameters")
	}
	return true, "", nil
}

const quantSMA20ParameterSchemaDocument = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:kasign:researchflow:quant-sma20-parameters:1.0.0",
  "title": "Quant SMA20 Parameters",
  "type": "object",
  "properties": {
    "symbol": {"type": "string", "pattern": "^[0-9]{6}\\.(SH|SZ|BJ)$"},
    "start_date": {"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
    "end_date": {"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
    "limit": {"type": "integer", "minimum": 20, "maximum": 1000}
  },
  "required": ["symbol", "start_date", "end_date", "limit"],
  "additionalProperties": false
}`

// quantFactorParameterSchemaDocument 是 Quant Factor 参数的冻结定义。
// limit 下限取三因子中最小算法窗口（momentum_5d 的 6 bars）；逐因子的精确
// 最小样本数由 Quant Stage Adapter 与 Registry Entry 判定，Schema 只做粗筛。
const quantFactorParameterSchemaDocument = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:kasign:researchflow:quant-factor-parameters:1.0.0",
  "title": "Quant Factor Parameters",
  "type": "object",
  "properties": {
    "factor_code": {
      "type": "string",
      "enum": ["ma20_distance", "momentum_5d", "momentum_20d"]
    },
    "symbol": {"type": "string", "pattern": "^[0-9]{6}\\.(SH|SZ|BJ)$"},
    "start_date": {"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
    "end_date": {"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
    "limit": {"type": "integer", "minimum": 6, "maximum": 1000}
  },
  "required": ["factor_code", "symbol", "start_date", "end_date", "limit"],
  "additionalProperties": false
}`

// quantTrendParameterSchemaDocument 是 Quant Trend 参数的冻结定义。
// limit 下限取 D1 周期集合的最长窗口（momentum_60d 的 61 bars）；方向判定周期
// 由决议固定，因此参数中不存在 window/period 字段，调用方无法改变周期集合。
const quantTrendParameterSchemaDocument = `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:kasign:researchflow:quant-trend-parameters:1.0.0",
  "title": "Quant Trend Parameters",
  "type": "object",
  "properties": {
    "symbol": {"type": "string", "pattern": "^[0-9]{6}\\.(SH|SZ|BJ)$"},
    "start_date": {"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
    "end_date": {"type": "string", "pattern": "^[0-9]{4}-[0-9]{2}-[0-9]{2}$"},
    "limit": {"type": "integer", "minimum": 61, "maximum": 1000}
  },
  "required": ["symbol", "start_date", "end_date", "limit"],
  "additionalProperties": false
}`

type researchflowParameterSchemaRegistry struct{}

func (researchflowParameterSchemaRegistry) Get(ctx context.Context, ref string) (parameterschema.Definition, error) {
	if ctx == nil {
		return parameterschema.Definition{}, errors.New("Researchflow parameter schema context is required")
	}
	if err := ctx.Err(); err != nil {
		return parameterschema.Definition{}, err
	}
	switch ref {
	case newsSearchParameterSchemaRef:
		return newsSearchParameterSchemaRegistry{}.Get(ctx, ref)
	case newsFeedParameterSchemaRef:
		return parameterschema.Definition{
			Ref: newsFeedParameterSchemaRef, Version: newsFeedParameterSchemaVersion,
			Document: json.RawMessage(newsFeedParameterSchemaDocument),
		}, nil
	case quantSMA20ParameterSchemaRef:
		return parameterschema.Definition{
			Ref: quantSMA20ParameterSchemaRef, Version: quantSMA20ParameterSchemaVersion,
			Document: json.RawMessage(quantSMA20ParameterSchemaDocument),
		}, nil
	case quantFactorParameterSchemaRef:
		return parameterschema.Definition{
			Ref: quantFactorParameterSchemaRef, Version: quantFactorParameterSchemaVersion,
			Document: json.RawMessage(quantFactorParameterSchemaDocument),
		}, nil
	case quantTrendParameterSchemaRef:
		return parameterschema.Definition{
			Ref: quantTrendParameterSchemaRef, Version: quantTrendParameterSchemaVersion,
			Document: json.RawMessage(quantTrendParameterSchemaDocument),
		}, nil
	default:
		return parameterschema.Definition{}, fmt.Errorf("%w: %s", parameterschema.ErrSchemaNotFound, ref)
	}
}

func newResearchflowParameterSchemaValidator() (*parameterschema.Validator, error) {
	return parameterschema.NewValidator(researchflowParameterSchemaRegistry{})
}

type quantSMA20ApplicabilityEvaluator struct{}

func (quantSMA20ApplicabilityEvaluator) Evaluate(
	ctx context.Context,
	rule template.ApplicabilityRule,
	parameters json.RawMessage,
) (bool, string, error) {
	if ctx == nil {
		return false, "", errors.New("Quant SMA20 applicability context is required")
	}
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	if rule.RuleRef != quantSMA20ApplicabilityRuleRef || rule.RuleVersion != quantSMA20ApplicabilityVersion {
		return false, "", fmt.Errorf("unsupported Quant SMA20 applicability rule: %s@%s", rule.RuleRef, rule.RuleVersion)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &object); err != nil || object == nil {
		return false, "", errors.New("Quant SMA20 applicability requires validated object parameters")
	}
	return true, "", nil
}

type quantFactorApplicabilityEvaluator struct{}

func (quantFactorApplicabilityEvaluator) Evaluate(
	ctx context.Context,
	rule template.ApplicabilityRule,
	parameters json.RawMessage,
) (bool, string, error) {
	if ctx == nil {
		return false, "", errors.New("Quant Factor applicability context is required")
	}
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	if rule.RuleRef != quantFactorApplicabilityRuleRef || rule.RuleVersion != quantFactorApplicabilityVersion {
		return false, "", fmt.Errorf("unsupported Quant Factor applicability rule: %s@%s", rule.RuleRef, rule.RuleVersion)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &object); err != nil || object == nil {
		return false, "", errors.New("Quant Factor applicability requires validated object parameters")
	}
	return true, "", nil
}

type quantTrendApplicabilityEvaluator struct{}

func (quantTrendApplicabilityEvaluator) Evaluate(
	ctx context.Context,
	rule template.ApplicabilityRule,
	parameters json.RawMessage,
) (bool, string, error) {
	if ctx == nil {
		return false, "", errors.New("Quant Trend applicability context is required")
	}
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	if rule.RuleRef != quantTrendApplicabilityRuleRef || rule.RuleVersion != quantTrendApplicabilityVersion {
		return false, "", fmt.Errorf("unsupported Quant Trend applicability rule: %s@%s", rule.RuleRef, rule.RuleVersion)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &object); err != nil || object == nil {
		return false, "", errors.New("Quant Trend applicability requires validated object parameters")
	}
	return true, "", nil
}

type researchflowApplicabilityEvaluator struct{}

func (researchflowApplicabilityEvaluator) Evaluate(
	ctx context.Context,
	rule template.ApplicabilityRule,
	parameters json.RawMessage,
) (bool, string, error) {
	switch rule.RuleRef {
	case newsSearchApplicabilityRuleRef:
		return (newsSearchApplicabilityEvaluator{}).Evaluate(ctx, rule, parameters)
	case quantSMA20ApplicabilityRuleRef:
		return (quantSMA20ApplicabilityEvaluator{}).Evaluate(ctx, rule, parameters)
	case quantFactorApplicabilityRuleRef:
		return (quantFactorApplicabilityEvaluator{}).Evaluate(ctx, rule, parameters)
	case quantTrendApplicabilityRuleRef:
		return (quantTrendApplicabilityEvaluator{}).Evaluate(ctx, rule, parameters)
	case newsFeedApplicabilityRuleRef:
		return (newsFeedApplicabilityEvaluator{}).Evaluate(ctx, rule, parameters)
	default:
		return false, "", fmt.Errorf("unsupported Researchflow applicability rule: %s@%s", rule.RuleRef, rule.RuleVersion)
	}
}

func newQuantSMA20ParameterSchemaValidator() (*parameterschema.Validator, error) {
	return parameterschema.NewValidator(researchflowParameterSchemaRegistry{})
}

type newsFeedApplicabilityEvaluator struct{}

func (newsFeedApplicabilityEvaluator) Evaluate(
	ctx context.Context,
	rule template.ApplicabilityRule,
	parameters json.RawMessage,
) (bool, string, error) {
	if ctx == nil {
		return false, "", errors.New("News Feed applicability context is required")
	}
	if err := ctx.Err(); err != nil {
		return false, "", err
	}
	if rule.RuleRef != newsFeedApplicabilityRuleRef || rule.RuleVersion != newsFeedApplicabilityVersion {
		return false, "", fmt.Errorf("unsupported News Feed applicability rule: %s@%s", rule.RuleRef, rule.RuleVersion)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &object); err != nil || object == nil || len(object) != 0 {
		return false, "", errors.New("News Feed applicability requires an empty validated parameter object")
	}
	return true, "", nil
}
