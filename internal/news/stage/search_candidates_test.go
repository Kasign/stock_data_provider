package stage

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type testProvider struct {
	code  string
	items []domain.Candidate
	err   error
}

func (p testProvider) Code() string { return p.code }
func (p testProvider) Search(context.Context, domain.SearchRequest) ([]domain.Candidate, error) {
	return p.items, p.err
}

func TestSearchCandidatesAdapterBuildsPayloadAndMeasuredQualitySummary(t *testing.T) {
	published := time.Date(2026, 11, 16, 1, 2, 3, 0, time.UTC)
	search := app.NewSearchCandidates(app.StaticProviderSet{Providers: []app.Provider{
		testProvider{code: "provider_a", items: []domain.Candidate{{
			Title: "headline", RawURL: "https://publisher.example/item",
			PublishedAt: &published,
		}}},
		testProvider{code: "provider_b", err: &app.ProviderError{Code: app.ProviderErrorTimeout, Retryable: true}},
	}})
	adapter, err := NewSearchCandidatesAdapter(search)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.Execute(context.Background(), domain.SearchRequest{Query: "market", QueryDimension: "market", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got.OutputType != SearchCandidatesOutputType || got.SchemaVersion != SearchCandidatesSchemaVersion {
		t.Fatalf("discriminator mismatch: %+v", got)
	}
	var payload SearchCandidatesOutput
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != domain.SearchPartial || len(payload.Candidates) != 1 || len(payload.Outcomes) != 2 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	if payload.Candidates[0].ProviderCode != "provider_a" || payload.Candidates[0].ObservedAt.IsZero() {
		t.Fatalf("candidate attribution/observation missing: %+v", payload.Candidates[0])
	}
	if payload.Outcomes[1].Status != domain.ProviderFailed || !payload.Outcomes[1].Retryable {
		t.Fatalf("provider outcome was not preserved: %+v", payload.Outcomes[1])
	}
	var summary SearchCandidatesQualitySummary
	if err := json.Unmarshal(got.QualitySummary, &summary); err != nil {
		t.Fatal(err)
	}
	if summary.ProviderCount != 2 || summary.ProviderSuccessCount != 1 ||
		summary.ProviderFailedCount != 1 || summary.CandidateCount != 1 {
		t.Fatalf("unexpected quality summary: %+v", summary)
	}
	if summary.CandidateMissingFieldCount["publisher"] != 1 {
		t.Fatalf("missing-field counts mismatch: %+v", summary.CandidateMissingFieldCount)
	}
}

func TestSearchCandidatesAdapterDoesNotTurnUnconfiguredProvidersIntoEmptySuccess(t *testing.T) {
	adapter, err := NewSearchCandidatesAdapter(app.NewSearchCandidates(app.StaticProviderSet{}))
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.Execute(context.Background(), domain.SearchRequest{Query: "market", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var payload SearchCandidatesOutput
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	var summary SearchCandidatesQualitySummary
	if err := json.Unmarshal(got.QualitySummary, &summary); err != nil {
		t.Fatal(err)
	}
	if payload.Status != domain.SearchSkipped || payload.FailureCode != "no_provider_configured" {
		t.Fatalf("unconfigured providers must remain skipped: %+v", payload)
	}
	if summary.ProviderCount != 0 || summary.ProviderSkippedCount != 0 {
		t.Fatalf("unconfigured providers must not be counted as empty results: %+v", summary)
	}
}

func TestSearchCandidatesAdapterRejectsMissingUseCase(t *testing.T) {
	if _, err := NewSearchCandidatesAdapter(nil); !errors.Is(err, ErrSearchUseCaseRequired) {
		t.Fatalf("expected missing use case error, got %v", err)
	}
}

func TestDecodeParametersRejectsUnknownFieldsAndTrailingValues(t *testing.T) {
	for _, raw := range []string{
		`{"query":"market","limit":10,"api_key":"must-not-be-accepted"}`,
		`{"query":"market","limit":10} {}`,
	} {
		if _, err := DecodeParameters(json.RawMessage(raw)); err == nil {
			t.Fatalf("expected invalid parameters for %s", raw)
		}
	}
	request, err := DecodeParameters(json.RawMessage(`{"query":"market","query_dimension":"market","limit":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if request.Query != "market" || request.QueryDimension != "market" || request.Limit != 10 {
		t.Fatalf("decoded request mismatch: %+v", request)
	}
}

func TestSearchCandidatesOutputQualitySummaryGolden(t *testing.T) {
	raw, err := json.Marshal(SearchCandidatesQualitySummary{
		SearchStatus: domain.SearchEmpty, ProviderCount: 1, ProviderEmptyCount: 1,
		CandidateMissingFieldCount: map[string]int{},
	})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"search_status":"empty","provider_count":1,"provider_success_count":0,"provider_empty_count":1,"provider_failed_count":0,"provider_skipped_count":0,"candidate_count":0,"candidate_missing_field_count":{}}`
	if string(raw) != want {
		t.Fatalf("quality summary golden mismatch:\n got %s\nwant %s", raw, want)
	}
}

func TestValidatePayloadJSONAcceptsAdapterOutputAndRejectsUnknownFields(t *testing.T) {
	raw, err := json.Marshal(SearchCandidatesOutput{
		Status: domain.SearchSkipped, FailureCode: "no_provider_configured",
		Candidates: []Candidate{}, Outcomes: []ProviderOutcome{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePayloadJSON(raw); err != nil {
		t.Fatalf("valid payload rejected: %v", err)
	}
	if err := ValidatePayloadJSON(json.RawMessage(`{"status":"skipped","failure_code":"","candidates":[],"outcomes":[],"unexpected":true}`)); err == nil {
		t.Fatal("unknown payload field must be rejected")
	}
	if err := ValidatePayloadJSON(json.RawMessage(`{"status":"skipped","failure_code":"","candidates":[]}`)); err == nil {
		t.Fatal("missing required payload field must be rejected")
	}
}

func TestValidatePayloadJSONRejectsInvalidCandidateAndProviderOutcome(t *testing.T) {
	observed := time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC)
	payload := SearchCandidatesOutput{
		Status: domain.SearchSuccess, Candidates: []Candidate{{
			ObservedAt: observed, MissingFields: []string{"invented_field"},
		}}, Outcomes: []ProviderOutcome{},
	}
	raw, _ := json.Marshal(payload)
	if err := ValidatePayloadJSON(raw); err == nil {
		t.Fatal("unknown missing-field reason must be rejected")
	}
	payload.Candidates[0].MissingFields = []string{}
	payload.Outcomes = []ProviderOutcome{{Status: domain.ProviderSuccess, CandidateCount: 0, DurationNS: -1, ObservedAt: observed}}
	raw, _ = json.Marshal(payload)
	if err := ValidatePayloadJSON(raw); err == nil {
		t.Fatal("negative provider duration must be rejected")
	}
}

func TestValidateQualitySummaryJSONEnforcesProviderCountReconciliation(t *testing.T) {
	valid := json.RawMessage(`{"search_status":"partial","provider_count":2,"provider_success_count":1,"provider_empty_count":0,"provider_failed_count":1,"provider_skipped_count":0,"candidate_count":3,"candidate_missing_field_count":{"publisher":1}}`)
	if err := ValidateQualitySummaryJSON(valid); err != nil {
		t.Fatalf("valid quality summary rejected: %v", err)
	}
	invalid := json.RawMessage(`{"search_status":"partial","provider_count":2,"provider_success_count":1,"provider_empty_count":0,"provider_failed_count":0,"provider_skipped_count":0,"candidate_count":3,"candidate_missing_field_count":{}}`)
	if err := ValidateQualitySummaryJSON(invalid); err == nil {
		t.Fatal("provider status counts must reconcile to provider_count")
	}
}

func TestSearchCandidatesGoldenMatchesSchemas(t *testing.T) {
	goldenRaw, err := os.ReadFile("../../../doc/news/golden/NEWS-STAGE-search-candidates-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Payload        json.RawMessage `json:"payload"`
		QualitySummary json.RawMessage `json:"quality_summary"`
	}
	if err := json.Unmarshal(goldenRaw, &golden); err != nil {
		t.Fatal(err)
	}
	if err := ValidatePayloadJSON(golden.Payload); err != nil {
		t.Fatalf("golden payload rejected: %v", err)
	}
	if err := ValidateQualitySummaryJSON(golden.QualitySummary); err != nil {
		t.Fatalf("golden quality summary rejected: %v", err)
	}
	validateSchema := func(path string, raw json.RawMessage) {
		t.Helper()
		schemaRaw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var document any
		if err := json.Unmarshal(schemaRaw, &document); err != nil {
			t.Fatal(err)
		}
		object, ok := document.(map[string]any)
		if !ok {
			t.Fatalf("schema %s must be an object", path)
		}
		id, ok := object["$id"].(string)
		if !ok || id == "" {
			t.Fatalf("schema %s has no $id", path)
		}
		compiler := jsonschema.NewCompiler()
		if err := compiler.AddResource(id, document); err != nil {
			t.Fatal(err)
		}
		compiled, err := compiler.Compile(id)
		if err != nil {
			t.Fatal(err)
		}
		var instance any
		if err := json.Unmarshal(raw, &instance); err != nil {
			t.Fatal(err)
		}
		if err := compiled.Validate(instance); err != nil {
			t.Fatalf("schema %s rejected golden: %v", path, err)
		}
	}
	validateSchema("../../../doc/news/schema/NEWS-STAGE-search-candidates-payload-1.0.0.schema.json", golden.Payload)
	validateSchema("../../../doc/news/schema/NEWS-STAGE-search-candidates-quality-summary-1.0.0.schema.json", golden.QualitySummary)
}
