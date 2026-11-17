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
