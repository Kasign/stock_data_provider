package template

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gowebpki/jcs"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
)

type PropagationPolicy struct {
	OnDegraded domain.Propagation `json:"on_degraded"`
	OnSkipped  domain.Propagation `json:"on_skipped"`
	OnFailed   domain.Propagation `json:"on_failed"`
	OnTimedOut domain.Propagation `json:"on_timed_out"`
	OnBlocked  domain.Propagation `json:"on_blocked"`
}
type BackoffPolicy struct {
	Algorithm   string `json:"algorithm"`
	BaseDelayMS int64  `json:"base_delay_ms"`
	MaxDelayMS  int64  `json:"max_delay_ms"`
}
type RetryPolicy struct {
	Ref                  string        `json:"retry_policy_ref"`
	MaxAttempts          int           `json:"max_attempts"`
	RetryableReasonCodes []string      `json:"retryable_reason_codes"`
	Backoff              BackoffPolicy `json:"backoff_policy"`
	AttemptTimeoutMS     int64         `json:"attempt_timeout_ms"`
}
type ApplicabilityRule struct {
	RuleVersion string `json:"rule_version"`
	RuleRef     string `json:"rule_ref"`
}
type StageDefinition struct {
	Code          string            `json:"stage_code"`
	Required      bool              `json:"required"`
	Dependencies  []string          `json:"dependencies"`
	Priority      int               `json:"priority"`
	Applicability ApplicabilityRule `json:"applicability"`
	Retry         RetryPolicy       `json:"retry_policy"`
	Propagation   PropagationPolicy `json:"propagation"`
	GateType      string            `json:"gate_type,omitempty"`
}

// Parameter scope values. The zero value means ParameterScopeTemplate so that
// every template published before this field existed keeps its exact
// serialized bytes and therefore its exact fingerprint.
const (
	// ParameterScopeTemplate passes the single parameter document verbatim to
	// every Stage. This is the historical behaviour.
	ParameterScopeTemplate = "template"
	// ParameterScopePerStage requires one envelope document
	// {"stages":{"<stage_code>":{...}}} and delivers only the addressed subtree
	// to each Stage.
	ParameterScopePerStage = "per_stage"
)

type WorkflowTemplate struct {
	Ref                   string            `json:"template_ref"`
	Version               string            `json:"version"`
	RequiresHardRiskGate  bool              `json:"requires_hard_risk_gate"`
	AllowEmptyInputs      bool              `json:"allow_empty_inputs"`
	ParameterSchemaRef    string            `json:"parameter_schema_ref"`
	ParameterScope        string            `json:"parameter_scope,omitempty"`
	OutputRegistryVersion string            `json:"output_registry_version"`
	ResourcePolicyRef     string            `json:"resource_policy_ref"`
	Stages                []StageDefinition `json:"stages"`
	Fingerprint           string            `json:"fingerprint,omitempty"`
}
type ApplicabilityEvaluator interface {
	Evaluate(context.Context, ApplicabilityRule, json.RawMessage) (bool, string, error)
}
type HardRiskGateRegistry interface{ Contains(stageCode string) bool }
type ResolvedStage struct {
	Definition    StageDefinition
	Dependencies  []string
	InitialStatus domain.StageStatus
	ReasonCodes   []string
}
type ParameterSchemaSnapshot struct {
	Ref         string
	Version     string
	Fingerprint string
}

type ParameterSchemaValidator interface {
	Validate(context.Context, string, json.RawMessage) (ParameterSchemaSnapshot, error)
}

type ResolvedTemplate struct {
	Ref                   string
	Version               string
	TemplateFingerprint   string `json:"template_fingerprint"`
	Fingerprint           string
	RequiresHardRiskGate  bool
	AllowEmptyInputs      bool
	ParameterScope        string `json:"parameter_scope,omitempty"`
	OutputRegistryVersion string
	ResourcePolicyRef     string
	ResourcePolicy        resourcepolicy.Snapshot `json:"resource_policy_snapshot"`
	ParameterSchema       ParameterSchemaSnapshot
	Stages                []ResolvedStage
	Excluded              []domain.ExcludedStage
}

var ErrInvalidTemplate = errors.New("invalid workflow template")

const maxRetryDelayMS = int64(1<<63-1) / int64(time.Millisecond)

func Validate(t WorkflowTemplate, gates HardRiskGateRegistry) error {
	if strings.TrimSpace(t.Ref) == "" || strings.TrimSpace(t.Version) == "" || len(t.Stages) == 0 ||
		strings.TrimSpace(t.ParameterSchemaRef) == "" || strings.TrimSpace(t.OutputRegistryVersion) == "" ||
		strings.TrimSpace(t.ResourcePolicyRef) == "" {
		return fmt.Errorf("%w: template identity, schemas, resource policy and stages are required", ErrInvalidTemplate)
	}
	// An explicit scope is required so that parameter routing never depends on
	// sniffing the shape of a business document.
	switch t.ParameterScope {
	case "", ParameterScopeTemplate, ParameterScopePerStage:
	default:
		return fmt.Errorf("%w: unsupported parameter scope %q", ErrInvalidTemplate, t.ParameterScope)
	}
	byCode := make(map[string]StageDefinition, len(t.Stages))
	for _, stage := range t.Stages {
		if stage.GateType != "" && stage.GateType != "hard_risk" {
			return fmt.Errorf("%w: stage %s has unsupported gate type %q", ErrInvalidTemplate, stage.Code, stage.GateType)
		}
		if strings.TrimSpace(stage.Code) == "" || strings.TrimSpace(stage.Applicability.RuleRef) == "" ||
			strings.TrimSpace(stage.Applicability.RuleVersion) == "" || strings.TrimSpace(stage.Retry.Ref) == "" {
			return fmt.Errorf("%w: stage code, applicability rule and retry policy reference are required", ErrInvalidTemplate)
		}
		if _, exists := byCode[stage.Code]; exists {
			return fmt.Errorf("%w: duplicate stage code %q", ErrInvalidTemplate, stage.Code)
		}
		depSeen := map[string]struct{}{}
		for _, dep := range stage.Dependencies {
			if _, exists := depSeen[dep]; exists {
				return fmt.Errorf("%w: duplicate dependency %q in stage %s", ErrInvalidTemplate, dep, stage.Code)
			}
			depSeen[dep] = struct{}{}
		}
		if err := validateRetry(stage.Retry); err != nil {
			return fmt.Errorf("%w: stage %s: %v", ErrInvalidTemplate, stage.Code, err)
		}
		if err := validatePropagation(stage.Propagation); err != nil {
			return fmt.Errorf("%w: stage %s: %v", ErrInvalidTemplate, stage.Code, err)
		}
		if stage.Required && stage.Propagation.OnSkipped == domain.PropagateNoImpact {
			return fmt.Errorf("%w: required stage %s cannot use skipped=no_impact", ErrInvalidTemplate, stage.Code)
		}
		byCode[stage.Code] = stage
	}
	declaredGates := 0
	for _, stage := range t.Stages {
		if stage.GateType == "hard_risk" {
			declaredGates++
		}
		for _, dependency := range stage.Dependencies {
			if dependency == stage.Code {
				return fmt.Errorf("%w: stage %s depends on itself", ErrInvalidTemplate, stage.Code)
			}
			if _, ok := byCode[dependency]; !ok {
				return fmt.Errorf("%w: stage %s references unknown dependency %s", ErrInvalidTemplate, stage.Code, dependency)
			}
		}
		if stage.GateType == "hard_risk" {
			if gates == nil || !gates.Contains(stage.Code) {
				return fmt.Errorf("%w: hard risk stage %s is not registered", ErrInvalidTemplate, stage.Code)
			}
			p := stage.Propagation
			if !stage.Required || p.OnDegraded != domain.PropagateFail || p.OnSkipped != domain.PropagateFail ||
				p.OnFailed != domain.PropagateFail || p.OnTimedOut != domain.PropagateFail || p.OnBlocked != domain.PropagateFail {
				return fmt.Errorf("%w: hard risk stage %s must be required and fail closed", ErrInvalidTemplate, stage.Code)
			}
		}
	}
	if t.RequiresHardRiskGate && declaredGates == 0 {
		return fmt.Errorf("%w: executable template must declare a hard risk gate", ErrInvalidTemplate)
	}
	if _, err := topologicalOrder(t.Stages); err != nil {
		return err
	}
	return nil
}

func validateRetry(p RetryPolicy) error {
	if p.MaxAttempts < 1 || p.AttemptTimeoutMS <= 0 || p.Backoff.Algorithm != "exponential_full_jitter_v1" ||
		p.Backoff.BaseDelayMS <= 0 || p.Backoff.MaxDelayMS < p.Backoff.BaseDelayMS ||
		p.Backoff.MaxDelayMS > maxRetryDelayMS || p.AttemptTimeoutMS > maxRetryDelayMS {
		return errors.New("retry policy must declare bounded attempts, positive timeout and exponential_full_jitter_v1 backoff")
	}
	seen := map[string]struct{}{}
	for _, code := range p.RetryableReasonCodes {
		if strings.TrimSpace(code) == "" || strings.Contains(code, "*") {
			return errors.New("retryable reason codes must be exact non-empty codes")
		}
		if _, ok := seen[code]; ok {
			return fmt.Errorf("duplicate retryable reason code %q", code)
		}
		seen[code] = struct{}{}
	}
	return nil
}
func validatePropagation(p PropagationPolicy) error {
	if p.OnDegraded != domain.PropagateAccept && p.OnDegraded != domain.PropagatePartial && p.OnDegraded != domain.PropagateFail {
		return errors.New("invalid on_degraded policy")
	}
	if p.OnSkipped != domain.PropagateNoImpact && p.OnSkipped != domain.PropagatePartial && p.OnSkipped != domain.PropagateFail {
		return errors.New("invalid on_skipped policy")
	}
	if p.OnFailed != domain.PropagatePartial && p.OnFailed != domain.PropagateFail {
		return errors.New("invalid on_failed policy")
	}
	if p.OnTimedOut != domain.PropagatePartial && p.OnTimedOut != domain.PropagateFail {
		return errors.New("invalid on_timed_out policy")
	}
	if p.OnBlocked != domain.PropagatePartial && p.OnBlocked != domain.PropagateFail {
		return errors.New("invalid on_blocked policy")
	}
	return nil
}

// Fingerprint includes all execution-semantic template fields; display-only metadata is intentionally absent.
func Fingerprint(t WorkflowTemplate) (string, error) {
	t.Fingerprint = ""
	t.Stages = append([]StageDefinition(nil), t.Stages...)
	sort.Slice(t.Stages, func(i, j int) bool { return t.Stages[i].Code < t.Stages[j].Code })
	for i := range t.Stages {
		t.Stages[i].Dependencies = append([]string(nil), t.Stages[i].Dependencies...)
		t.Stages[i].Retry.RetryableReasonCodes = append([]string(nil), t.Stages[i].Retry.RetryableReasonCodes...)
		sort.Strings(t.Stages[i].Dependencies)
		sort.Strings(t.Stages[i].Retry.RetryableReasonCodes)
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return "", fmt.Errorf("encode workflow template: %w", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", fmt.Errorf("canonicalize workflow template: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

func resolvedFingerprint(templateFingerprint string, schema ParameterSchemaSnapshot, resourcePolicyFingerprint string) (string, error) {
	payload := struct {
		TemplateFingerprint        string `json:"template_fingerprint"`
		ParameterSchemaRef         string `json:"parameter_schema_ref"`
		ParameterSchemaVersion     string `json:"parameter_schema_version"`
		ParameterSchemaFingerprint string `json:"parameter_schema_fingerprint"`
		ResourcePolicyFingerprint  string `json:"resource_policy_fingerprint"`
	}{templateFingerprint, schema.Ref, schema.Version, schema.Fingerprint, resourcePolicyFingerprint}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode resolved template fingerprint: %w", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return "", fmt.Errorf("canonicalize resolved template fingerprint: %w", err)
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// BindResourcePolicySnapshot binds the immutable resource inventory selected at Run creation.
// The policy definition fingerprint participates in identity; the dynamic inventory is persisted
// for execution but deliberately excluded from that identity.
func BindResourcePolicySnapshot(resolved ResolvedTemplate, snapshot resourcepolicy.Snapshot) (ResolvedTemplate, error) {
	if snapshot.PolicyRef == "" || snapshot.PolicyRef != resolved.ResourcePolicyRef ||
		len(snapshot.PolicyFingerprint) != 64 || snapshot.PolicyFingerprint != strings.ToLower(snapshot.PolicyFingerprint) ||
		resourcepolicy.ValidateSnapshotInventory(snapshot) != nil {
		return ResolvedTemplate{}, fmt.Errorf("%w: resource policy snapshot identity or inventory is invalid", ErrInvalidTemplate)
	}
	if _, err := hex.DecodeString(snapshot.PolicyFingerprint); err != nil {
		return ResolvedTemplate{}, fmt.Errorf("%w: resource policy fingerprint must be SHA-256 hex", ErrInvalidTemplate)
	}
	snapshot.AllowedProviderCodes = append([]string(nil), snapshot.AllowedProviderCodes...)
	snapshot.AllowedSourceEndpointRefs = append([]string(nil), snapshot.AllowedSourceEndpointRefs...)
	resolved.ResourcePolicy = snapshot
	if resolved.TemplateFingerprint == "" {
		return ResolvedTemplate{}, fmt.Errorf("%w: resolved template fingerprint is missing", ErrInvalidTemplate)
	}
	fingerprint, err := resolvedFingerprint(resolved.TemplateFingerprint, resolved.ParameterSchema, snapshot.PolicyFingerprint)
	if err != nil {
		return ResolvedTemplate{}, err
	}
	resolved.Fingerprint = fingerprint
	return resolved, nil
}

var ErrStageParameters = errors.New("workflow stage parameters are invalid")

// SplitPerStageParameters parses the per-Stage parameter envelope and returns
// the exact subtree bytes addressed to each Stage code. It is the single
// definition of the envelope shape, shared by CreateRun validation and Worker
// dispatch, so the two can never disagree about what a subtree is.
//
// The returned values are copies of the envelope bytes: callers cannot mutate
// the persisted document, and byte identity with the envelope is preserved for
// downstream content-addressed validation.
func SplitPerStageParameters(parameters json.RawMessage) (map[string]json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(parameters, &root); err != nil {
		return nil, fmt.Errorf("%w: per-Stage parameters must be a JSON object", ErrStageParameters)
	}
	if len(root) != 1 {
		return nil, fmt.Errorf("%w: per-Stage parameters must contain exactly the stages key", ErrStageParameters)
	}
	rawStages, ok := root["stages"]
	if !ok {
		return nil, fmt.Errorf("%w: per-Stage parameters must contain exactly the stages key", ErrStageParameters)
	}
	if trimmed := bytes.TrimSpace(rawStages); len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("%w: stages must be a JSON object", ErrStageParameters)
	}
	var stages map[string]json.RawMessage
	if err := json.Unmarshal(rawStages, &stages); err != nil {
		return nil, fmt.Errorf("%w: stages must be a JSON object", ErrStageParameters)
	}
	split := make(map[string]json.RawMessage, len(stages))
	for code, raw := range stages {
		if strings.TrimSpace(code) == "" || strings.TrimSpace(code) != code {
			return nil, fmt.Errorf("%w: stage codes must be non-empty without surrounding whitespace", ErrStageParameters)
		}
		trimmed := bytes.TrimSpace(raw)
		// A non-empty object is required: an empty subtree is indistinguishable
		// from a caller that forgot to fill it in, and per-Stage dispatch has no
		// other signal to tell those apart.
		if len(trimmed) == 0 || trimmed[0] != '{' || bytes.Equal(trimmed, []byte("{}")) {
			return nil, fmt.Errorf("%w: stage %q parameters must be a non-empty JSON object", ErrStageParameters, code)
		}
		split[code] = json.RawMessage(append([]byte(nil), trimmed...))
	}
	return split, nil
}

// StageParameters returns the parameter document delivered to one Stage under
// the template's parameter scope.
func StageParameters(scope string, parameters json.RawMessage, stageCode string) (json.RawMessage, error) {
	if scope != ParameterScopePerStage {
		return append(json.RawMessage(nil), parameters...), nil
	}
	split, err := SplitPerStageParameters(parameters)
	if err != nil {
		return nil, err
	}
	subtree, ok := split[stageCode]
	if !ok {
		return nil, fmt.Errorf("%w: stage %q has no parameter subtree", ErrStageParameters, stageCode)
	}
	return subtree, nil
}

func cloneStageDefinition(stage StageDefinition) StageDefinition {
	stage.Dependencies = append([]string(nil), stage.Dependencies...)
	sort.Strings(stage.Dependencies)
	stage.Retry.RetryableReasonCodes = append([]string(nil), stage.Retry.RetryableReasonCodes...)
	sort.Strings(stage.Retry.RetryableReasonCodes)
	return stage
}

func Resolve(ctx context.Context, t WorkflowTemplate, parameters json.RawMessage, parameterSchemas ParameterSchemaValidator, evaluator ApplicabilityEvaluator, gates HardRiskGateRegistry) (ResolvedTemplate, error) {
	if err := Validate(t, gates); err != nil {
		return ResolvedTemplate{}, err
	}
	if parameterSchemas == nil {
		return ResolvedTemplate{}, fmt.Errorf("%w: parameter schema validator is required", ErrInvalidTemplate)
	}
	if evaluator == nil {
		return ResolvedTemplate{}, fmt.Errorf("%w: applicability evaluator is required", ErrInvalidTemplate)
	}
	// Validators and applicability evaluators are injected module boundaries.
	// Give each one an owned copy so an implementation cannot mutate the
	// caller's parameter bytes or influence a later Stage evaluation.
	parameterSnapshot := append(json.RawMessage(nil), parameters...)
	parameterSchema, err := parameterSchemas.Validate(ctx, t.ParameterSchemaRef, append(json.RawMessage(nil), parameterSnapshot...))
	if err != nil {
		return ResolvedTemplate{}, fmt.Errorf("%w: validate parameters against %q: %w", ErrInvalidTemplate, t.ParameterSchemaRef, err)
	}
	if parameterSchema.Ref != t.ParameterSchemaRef || strings.TrimSpace(parameterSchema.Version) == "" || len(parameterSchema.Fingerprint) != 64 {
		return ResolvedTemplate{}, fmt.Errorf("%w: parameter schema validator returned an incomplete or mismatched immutable snapshot", ErrInvalidTemplate)
	}
	if parameterSchema.Fingerprint != strings.ToLower(parameterSchema.Fingerprint) {
		return ResolvedTemplate{}, fmt.Errorf("%w: parameter schema fingerprint must be lowercase SHA-256 hex", ErrInvalidTemplate)
	}
	if _, err := hex.DecodeString(parameterSchema.Fingerprint); err != nil {
		return ResolvedTemplate{}, fmt.Errorf("%w: parameter schema fingerprint must be lowercase SHA-256 hex", ErrInvalidTemplate)
	}
	templateFingerprint, err := Fingerprint(t)
	if err != nil {
		return ResolvedTemplate{}, err
	}
	fingerprint, err := resolvedFingerprint(templateFingerprint, parameterSchema, "")
	if err != nil {
		return ResolvedTemplate{}, err
	}
	resolved := ResolvedTemplate{
		Ref: t.Ref, Version: t.Version, TemplateFingerprint: templateFingerprint, Fingerprint: fingerprint,
		RequiresHardRiskGate: t.RequiresHardRiskGate, AllowEmptyInputs: t.AllowEmptyInputs,
		ParameterScope:        t.ParameterScope,
		OutputRegistryVersion: t.OutputRegistryVersion, ResourcePolicyRef: t.ResourcePolicyRef,
		ParameterSchema: parameterSchema,
	}
	included := map[string]bool{}
	definitions := map[string]StageDefinition{}
	optionalSkipped := make(map[string]ResolvedStage)
	for _, stage := range t.Stages {
		// ResolvedTemplate is a persisted execution snapshot. It must not retain
		// caller-owned slice backing arrays from the mutable registry object.
		stage = cloneStageDefinition(stage)
		definitions[stage.Code] = stage
		ok, reason, evalErr := evaluator.Evaluate(ctx, stage.Applicability, append(json.RawMessage(nil), parameterSnapshot...))
		if evalErr != nil {
			return ResolvedTemplate{}, fmt.Errorf("evaluate applicability for %s: %w", stage.Code, evalErr)
		}
		if ok {
			included[stage.Code] = true
			continue
		}
		if strings.TrimSpace(reason) == "" {
			return ResolvedTemplate{}, fmt.Errorf("%w: non-applicable stage %s has no reason code", ErrInvalidTemplate, stage.Code)
		}
		if stage.Required {
			resolved.Excluded = append(resolved.Excluded, domain.ExcludedStage{StageCode: stage.Code, RuleVersion: stage.Applicability.RuleVersion, ReasonCode: reason})
			continue
		}
		// Optional non-applicable stages are materialized as terminal skipped rows.
		// They are not executable DAG nodes and therefore carry no runtime dependencies.
		optionalSkipped[stage.Code] = ResolvedStage{Definition: stage, InitialStatus: domain.StageSkipped, ReasonCodes: []string{reason}}
	}
	if len(included) == 0 {
		return ResolvedTemplate{}, fmt.Errorf("%w: resolved DAG must contain at least one applicable stage", ErrInvalidTemplate)
	}
	gateCount := 0
	active := make([]StageDefinition, 0, len(included))
	for code := range included {
		stage := definitions[code]
		if stage.GateType == "hard_risk" {
			gateCount++
		}
		for _, dep := range stage.Dependencies {
			if !included[dep] {
				return ResolvedTemplate{}, fmt.Errorf("%w: applicable stage %s depends on non-applicable stage %s", ErrInvalidTemplate, code, dep)
			}
		}
		active = append(active, stage)
	}
	if t.RequiresHardRiskGate && gateCount == 0 {
		return ResolvedTemplate{}, fmt.Errorf("%w: executable template has no applicable hard risk gate", ErrInvalidTemplate)
	}
	order, err := topologicalOrder(active)
	if err != nil {
		return ResolvedTemplate{}, err
	}
	for _, code := range order {
		stage := definitions[code]
		dependencies := append([]string(nil), stage.Dependencies...)
		sort.Strings(dependencies)
		stage.Dependencies = append([]string(nil), dependencies...)
		resolved.Stages = append(resolved.Stages, ResolvedStage{Definition: stage, Dependencies: dependencies, InitialStatus: domain.StagePending})
	}
	// Append optional skipped rows in stable code order. They are terminal at creation
	// and are deliberately excluded from the executable DAG's dependency ordering.
	skippedCodes := make([]string, 0, len(optionalSkipped))
	for code := range optionalSkipped {
		skippedCodes = append(skippedCodes, code)
	}
	sort.Strings(skippedCodes)
	for _, code := range skippedCodes {
		resolved.Stages = append(resolved.Stages, optionalSkipped[code])
	}
	sort.Slice(resolved.Excluded, func(i, j int) bool { return resolved.Excluded[i].StageCode < resolved.Excluded[j].StageCode })
	return resolved, nil
}

func topologicalOrder(stages []StageDefinition) ([]string, error) {
	byCode := make(map[string]StageDefinition, len(stages))
	indegree := make(map[string]int, len(stages))
	dependents := make(map[string][]string, len(stages))
	for _, stage := range stages {
		byCode[stage.Code] = stage
		indegree[stage.Code] = len(stage.Dependencies)
		for _, dep := range stage.Dependencies {
			dependents[dep] = append(dependents[dep], stage.Code)
		}
	}
	ready := []string{}
	for code, degree := range indegree {
		if degree == 0 {
			ready = append(ready, code)
		}
	}
	less := func(a, b string) bool {
		if byCode[a].Priority == byCode[b].Priority {
			return a < b
		}
		return byCode[a].Priority < byCode[b].Priority
	}
	sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
	order := make([]string, 0, len(stages))
	for len(ready) > 0 {
		code := ready[0]
		ready = ready[1:]
		order = append(order, code)
		for _, next := range dependents[code] {
			indegree[next]--
			if indegree[next] == 0 {
				ready = append(ready, next)
				sort.Slice(ready, func(i, j int) bool { return less(ready[i], ready[j]) })
			}
		}
	}
	if len(order) != len(byCode) {
		return nil, fmt.Errorf("%w: workflow DAG contains a cycle", ErrInvalidTemplate)
	}
	return order, nil
}
