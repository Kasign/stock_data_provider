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
	newsSearchParameterSchemaRef     = "doc/researchflow/schema/news-search-parameters-1.0.0.schema.json"
	newsSearchParameterSchemaVersion = "1.0.0"
	newsSearchApplicabilityRuleRef   = "news.search_candidates.always"
	newsSearchApplicabilityVersion   = "1.0.0"
	newsFeedParameterSchemaRef       = "doc/researchflow/schema/news-feed-collection-parameters-1.0.0.schema.json"
	newsFeedParameterSchemaVersion   = "1.0.0"
	newsFeedApplicabilityRuleRef     = "news.feed_collection.configured_sources"
	newsFeedApplicabilityVersion     = "1.0.0"
	quantSMA20ParameterSchemaRef     = "doc/researchflow/schema/quant-sma20-parameters-1.0.0.schema.json"
	quantSMA20ParameterSchemaVersion = "1.0.0"
	quantSMA20ApplicabilityRuleRef   = "quant.sma20.always"
	quantSMA20ApplicabilityVersion   = "1.0.0"
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
