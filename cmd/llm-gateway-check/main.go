// Command llm-gateway-check performs one real, non-streaming Responses call
// against the operator-configured LLM Gateway and reports whether the response
// carries the identity fields required by the complete-report projection.
//
// It deliberately never prints credentials, raw response bodies or endpoint
// URLs. The caller must configure SERVER_LLM_PROVIDER_JSON and
// SERVER_LLM_CAPABILITY_JSON plus the referenced secret before running it.
// No database or Redis connection is opened: this is an isolated Gateway probe.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/llm"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	"example.com/stock-analysis-flow/internal/server/llmtransport"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

const (
	probeReasonIdentityIncomplete = "llm_identity_incomplete"
	probeReasonStatusNotCompleted = "response_status_not_completed"
	probeReasonEmptyOutput        = "empty_output"
	probeReasonCallFailed         = "gateway_call_failed"
	probeReasonConfigMissing      = "llm_gateway_not_configured"
	probeRequestID                = "llm-gateway-check"
	probeCorrelationID            = "llm-gateway-check"
	probeMaxOutputTokens          = 64
)

// probeReport is a redacted verification result. It contains no credential,
// endpoint, raw response body, or upstream error detail.
type probeReport struct {
	Pass             bool
	Status           string
	ProviderID       string
	ModelRequested   string
	ModelActual      string
	ModelVersion     string
	ResponseID       string
	GatewayRequestID string
	OutputChars      int
	Reason           string
}

// buildProbeRequest builds the smallest request that is valid against the
// configured provider and current capability evidence. It asks for no tools
// and no structured output, so only the baseline responses_json capability is
// required.
func buildProbeRequest(provider llm.ProviderConfig, capabilities llm.CapabilityState, now time.Time) (llm.ModelRequest, error) {
	if err := provider.Validate(); err != nil {
		return llm.ModelRequest{}, err
	}
	if err := provider.ValidateCapabilityStateAt(capabilities, now); err != nil {
		return llm.ModelRequest{}, err
	}
	maxTokens := probeMaxOutputTokens
	if capabilities.MaxOutputTokensLimit > 0 && capabilities.MaxOutputTokensLimit < maxTokens {
		maxTokens = capabilities.MaxOutputTokensLimit
	}
	if maxTokens <= 0 {
		return llm.ModelRequest{}, errors.New("verified max_output_tokens_limit must be positive")
	}
	utcNow := now.UTC()
	return llm.ModelRequest{
		ProviderID:      provider.ProviderID,
		RequestID:       probeRequestID,
		CorrelationID:   probeCorrelationID,
		PurposeCode:     "gateway.probe",
		Model:           provider.DefaultModel,
		Instructions:    "Reply with the single word OK. Do not use tools.",
		InputText:       "Reply with OK.",
		MaxOutputTokens: maxTokens,
		DeadlineAt:      utcNow.Add(time.Duration(provider.RequestTimeoutMS) * time.Millisecond),
		SystemAsOf:      utcNow,
	}, nil
}

// evaluateProbe converts one Generate result into a redacted report. Any
// missing identity field is fail-closed because ComposeResult rejects the same
// fields in a complete-report projection. Error detail is reduced to an
// AdapterError safe message or a fixed reason, never the raw error text.
func evaluateProbe(response llm.ModelResponse, callErr error) probeReport {
	report := probeReport{
		Status:           string(response.Status),
		ProviderID:       response.ProviderID,
		ModelRequested:   response.ModelRequested,
		ModelActual:      response.ModelActual,
		ModelVersion:     response.ModelVersion,
		ResponseID:       response.ResponseID,
		GatewayRequestID: response.GatewayRequestID,
		OutputChars:      len([]rune(response.OutputText)),
	}
	if callErr != nil {
		var adapterErr *llm.AdapterError
		if errors.As(callErr, &adapterErr) && adapterErr.Info != nil &&
			strings.TrimSpace(adapterErr.Info.SafeMessage) != "" {
			report.Reason = adapterErr.Info.SafeMessage
		} else {
			report.Reason = probeReasonCallFailed
		}
		return report
	}
	if response.Status != llm.ResponseCompleted {
		report.Reason = probeReasonStatusNotCompleted
		return report
	}
	if strings.TrimSpace(response.ModelActual) == "" ||
		strings.TrimSpace(response.ModelVersion) == "" ||
		strings.TrimSpace(response.ResponseID) == "" {
		report.Reason = probeReasonIdentityIncomplete
		return report
	}
	if strings.TrimSpace(response.OutputText) == "" {
		report.Reason = probeReasonEmptyOutput
		return report
	}
	report.Pass = true
	return report
}

func run() int {
	if err := platformconfig.LoadDotEnvIfExists(".env"); err != nil {
		fmt.Fprintln(os.Stderr, "llm-gateway-check: load .env:", err)
		return 1
	}
	env := platformconfig.NewEnv()
	cfg, err := serverconfig.LoadLLM(env)
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm-gateway-check: invalid LLM configuration:", err)
		return 1
	}
	if !cfg.Configured {
		fmt.Fprintln(os.Stderr, "llm-gateway-check:", probeReasonConfigMissing,
			"— set SERVER_LLM_PROVIDER_JSON and SERVER_LLM_CAPABILITY_JSON plus the referenced secret")
		return 1
	}

	client, err := platformhttpclient.New(platformhttpclient.Config{
		Timeout: time.Duration(cfg.Provider.RequestTimeoutMS) * time.Millisecond,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm-gateway-check: construct Platform HTTP client:", err)
		return 1
	}
	resources := &serverresources.Resources{HTTPClient: client}
	defer resources.Close()

	adapter, err := llmtransport.NewResponsesAdapter(
		resources, cfg.Provider, cfg.APIKey, cfg.Capabilities, llm.SystemClock{}, cfg.Policy,
	)
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm-gateway-check: construct Gateway adapter:", err)
		return 1
	}
	now := time.Now().UTC()
	request, err := buildProbeRequest(cfg.Provider, cfg.Capabilities, now)
	if err != nil {
		fmt.Fprintln(os.Stderr, "llm-gateway-check: build probe request:", err)
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Provider.RequestTimeoutMS)*time.Millisecond)
	defer cancel()
	response, callErr := adapter.Generate(ctx, request)
	report := evaluateProbe(response, callErr)

	fmt.Printf("pass=%t status=%s model_requested=%s model_actual=%s model_version=%s response_id=%s gateway_request_id=%s output_chars=%d reason=%s\n",
		report.Pass, report.Status, report.ModelRequested, report.ModelActual,
		report.ModelVersion, report.ResponseID, report.GatewayRequestID,
		report.OutputChars, report.Reason)
	if report.Pass {
		fmt.Println("LLM 网关真实验收通过：响应身份字段完整，可用于完整报告的非权威解释投影。")
		return 0
	}
	fmt.Fprintln(os.Stderr, "LLM 网关真实验收未通过：响应缺失完整身份字段或调用失败；不会生成空壳报告。")
	return 1
}

func main() {
	os.Exit(run())
}
