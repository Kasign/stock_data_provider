package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	factorpkg "example.com/stock-analysis-flow/internal/quant/factor"
	quantresult "example.com/stock-analysis-flow/internal/quant/result"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

func resultReadFactorPayload(t *testing.T) quantresult.FactorPayload {
	t.Helper()
	systemAsOf, err := time.Parse(time.RFC3339, "2026-11-20T08:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	metric, err := factorpkg.NewDecimalQuantMetric(factorpkg.DecimalResult{
		Code: "ma20_distance", Value: "0.1234", Status: factorpkg.StatusAvailable, SampleCount: 20,
	}, systemAsOf, factorpkg.QualityValid, "1.0.0")
	if err != nil {
		t.Fatalf("build factor metric: %v", err)
	}
	payload, err := quantresult.NewFactorPayload(metric)
	if err != nil {
		t.Fatalf("build factor payload: %v", err)
	}
	return payload
}

func resultReadEnvelope(resultType string, payload quantresult.Payload) quantresult.Envelope {
	resultStatus := "completed"
	if resultType == "risk" {
		// 真实 CreateRiskService 把决策 reject 映射为 Manifest result_status=rejected。
		resultStatus = "rejected"
	}
	return quantresult.Envelope{
		SchemaVersion:        "1.0.0",
		ResultID:             "quant_result:sha256:" + strings.Repeat("a", 64),
		ResultType:           resultType,
		Scope:                "security",
		SecurityID:           "000001.SZ",
		SystemAsOf:           "2026-11-20T08:00:00Z",
		Period:               "20d",
		QualityStatus:        "valid",
		ResultStatus:         resultStatus,
		AuthorizationRef:     "quant_auth:sha256:" + strings.Repeat("b", 64),
		PayloadSchemaVersion: "1.0.0",
		Payload:              payload,
	}
}

func TestProjectQuantResultForReadExcludesRiskDecisionPayload(t *testing.T) {
	envelope := resultReadEnvelope("risk", nil)
	item, err := projectQuantResultForRead(envelope)
	if err != nil {
		t.Fatalf("project risk result: %v", err)
	}
	if len(item.Payload) != 0 {
		t.Fatalf("risk result must not expose RiskDecision payload, got %s", item.Payload)
	}
	if item.ResultID != envelope.ResultID || item.AuthorizationRef != envelope.AuthorizationRef {
		t.Fatalf("metadata projection = %+v", item)
	}
	var raw map[string]any
	encoded, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatal(err)
	}
	if _, exists := raw["reasons"]; exists {
		t.Fatal("risk projection leaked reasons")
	}
	if _, exists := raw["input_refs"]; exists {
		t.Fatal("risk projection leaked input_refs")
	}
}

func TestProjectQuantResultForReadIncludesAuthorizedFactorPayload(t *testing.T) {
	envelope := resultReadEnvelope("factor", resultReadFactorPayload(t))
	item, err := projectQuantResultForRead(envelope)
	if err != nil {
		t.Fatalf("project factor result: %v", err)
	}
	if len(item.Payload) == 0 {
		t.Fatal("factor payload must be projected")
	}
	var decoded map[string]any
	if err := json.Unmarshal(item.Payload, &decoded); err != nil {
		t.Fatalf("factor payload is not valid JSON: %v", err)
	}
	if decoded["metric_code"] != "ma20_distance" {
		t.Fatalf("factor payload = %s", item.Payload)
	}
}

func TestProjectQuantResultForReadRejectsUnsupportedType(t *testing.T) {
	if _, err := projectQuantResultForRead(resultReadEnvelope("unknown_type", nil)); err == nil {
		t.Fatal("unsupported result_type must be rejected")
	}
}

type resultReadQuantUseCaseStub struct {
	envelopes map[string]quantresult.Envelope
	errs      map[string]error
	calls     []string
}

func (stub *resultReadQuantUseCaseStub) Read(_ context.Context, _ string, resultID string) (quantresult.Envelope, error) {
	stub.calls = append(stub.calls, resultID)
	if err, failed := stub.errs[resultID]; failed {
		return quantresult.Envelope{}, err
	}
	envelope, ok := stub.envelopes[resultID]
	if !ok {
		return quantresult.Envelope{}, quantresult.ErrUnsupportedPayload
	}
	return envelope, nil
}

func TestCompositionReadQuantResultsForStagesFailClosed(t *testing.T) {
	ctx := context.Background()
	runRef := quantAssociationTestRunRef
	resultID := quantAssociationSMA20ResultID
	payload := json.RawMessage(quantAssociationSMA20Payload)

	tests := []struct {
		name       string
		stages     []researchflowdomain.Stage
		outputs    map[string]researchflowapplication.StageOutputResource
		outputErrs map[string]error
		readErr    error
		wantErr    error
		wantCalls  int
	}{
		{
			name: "unregistered stage is skipped",
			stages: []researchflowdomain.Stage{
				quantAssociationStageStage(runRef, "news.collect_feeds", researchflowdomain.StageSucceeded),
			},
			wantCalls: 0,
		},
		{
			name: "cross-run stage is integrity failure",
			stages: []researchflowdomain.Stage{
				quantAssociationStageStage("run:other", "quant.sma20", researchflowdomain.StageSucceeded),
			},
			wantErr:   researchflowapplication.ErrStageOutputIntegrity,
			wantCalls: 0,
		},
		{
			name: "failed stage is skipped",
			stages: []researchflowdomain.Stage{
				quantAssociationStageStage(runRef, "quant.sma20", researchflowdomain.StageFailed),
			},
			wantCalls: 0,
		},
		{
			name: "output read failure is fail-closed",
			stages: []researchflowdomain.Stage{
				quantAssociationStageStage(runRef, "quant.sma20", researchflowdomain.StageSucceeded),
			},
			outputErrs: map[string]error{
				"quant.sma20": researchflowapplication.ErrStageOutputUnavailable,
			},
			wantErr:   researchflowapplication.ErrStageOutputUnavailable,
			wantCalls: 0,
		},
		{
			name: "authorized result is read",
			stages: []researchflowdomain.Stage{
				quantAssociationStageStage(runRef, "quant.sma20", researchflowdomain.StageSucceeded),
			},
			outputs: map[string]researchflowapplication.StageOutputResource{
				"quant.sma20": {Content: payload},
			},
			wantCalls: 1,
		},
		{
			name: "authorized read failure propagates",
			stages: []researchflowdomain.Stage{
				quantAssociationStageStage(runRef, "quant.sma20", researchflowdomain.StageSucceeded),
			},
			outputs: map[string]researchflowapplication.StageOutputResource{
				"quant.sma20": {Content: payload},
			},
			readErr:   quantresult.ErrUnsupportedPayload,
			wantErr:   quantresult.ErrUnsupportedPayload,
			wantCalls: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outputs := &quantAssociationOutputReaderStub{resources: test.outputs, errs: test.outputErrs}
			reads := &resultReadQuantUseCaseStub{
				envelopes: map[string]quantresult.Envelope{
					resultID: resultReadEnvelope("indicator", nil),
				},
				errs: map[string]error{},
			}
			if test.readErr != nil {
				reads.errs[resultID] = test.readErr
			}
			composition := &researchflowComposition{
				stageOutputReader: outputs,
				quantResultRead:   reads,
			}
			envelopes, err := composition.readQuantResultsForStages(ctx, runRef, test.stages)
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(reads.calls) != test.wantCalls {
				t.Fatalf("authorized reads = %d, want %d", len(reads.calls), test.wantCalls)
			}
			if test.wantErr == nil && test.wantCalls == 1 && len(envelopes) != 1 {
				t.Fatalf("envelopes = %d, want 1", len(envelopes))
			}
		})
	}
}

type resultReadRunPortStub struct {
	result researchflowapplication.CreateRunResult
	err    error
	calls  int
	runRef string
}

func (stub *resultReadRunPortStub) GetRun(_ context.Context, runRef string) (researchflowapplication.CreateRunResult, error) {
	stub.calls++
	stub.runRef = runRef
	return stub.result, stub.err
}

func TestResultReadServiceAuthorizesBeforeReading(t *testing.T) {
	ctx := context.Background()
	runRef := quantAssociationTestRunRef
	errDenied := errors.New("run access denied")

	t.Run("authorization failure never reaches result reader", func(t *testing.T) {
		runs := &resultReadRunPortStub{err: errDenied}
		reads := &resultReadQuantUseCaseStub{}
		service := &researchflowRunResultReadService{
			runs: runs,
			reader: &researchflowComposition{
				stageOutputReader: &quantAssociationOutputReaderStub{},
				quantResultRead:   reads,
			},
		}
		if _, err := service.ReadRunResults(ctx, runRef); !errors.Is(err, errDenied) {
			t.Fatalf("error = %v, want %v", err, errDenied)
		}
		if len(reads.calls) != 0 {
			t.Fatalf("result reader was called %d times after authorization failure", len(reads.calls))
		}
	})

	t.Run("run ref mismatch is integrity failure", func(t *testing.T) {
		runs := &resultReadRunPortStub{
			result: researchflowapplication.CreateRunResult{
				Run: researchflowdomain.Run{Ref: "run:other"},
			},
		}
		reads := &resultReadQuantUseCaseStub{}
		service := &researchflowRunResultReadService{
			runs: runs,
			reader: &researchflowComposition{
				stageOutputReader: &quantAssociationOutputReaderStub{},
				quantResultRead:   reads,
			},
		}
		if _, err := service.ReadRunResults(ctx, runRef); !errors.Is(err, researchflowapplication.ErrStageOutputIntegrity) {
			t.Fatalf("error = %v, want ErrStageOutputIntegrity", err)
		}
		if len(reads.calls) != 0 {
			t.Fatalf("result reader was called %d times after run ref mismatch", len(reads.calls))
		}
	})
}

type resultReadHTTPPortStub struct {
	envelopes []quantresult.Envelope
	err       error
	calls     int
	runRef    string
}

func (stub *resultReadHTTPPortStub) ReadRunResults(_ context.Context, runRef string) ([]quantresult.Envelope, error) {
	stub.calls++
	stub.runRef = runRef
	return stub.envelopes, stub.err
}

func TestResultReadHTTPHandlerProjectsOnlyRestrictedFields(t *testing.T) {
	port := &resultReadHTTPPortStub{envelopes: []quantresult.Envelope{
		resultReadEnvelope("factor", resultReadFactorPayload(t)),
		resultReadEnvelope("risk", nil),
	}}
	handler := &researchflowHTTPHandler{resultRead: port}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/researchflow/runs/run-1/results", nil)
	request.SetPathValue("run_ref", "run-1")
	response := httptest.NewRecorder()
	handler.getResults(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
	var body struct {
		SchemaVersion string                       `json:"schema_version"`
		RunRef        string                       `json:"run_ref"`
		ResourceKind  string                       `json:"resource_kind"`
		Results       []researchflowResultReadItem `json:"results"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.SchemaVersion != "1.1.0" || body.RunRef != "run-1" {
		t.Fatalf("response envelope = %+v", body)
	}
	if body.ResourceKind != "authorized_domain_results" {
		t.Fatalf("resource_kind = %q must not claim full report", body.ResourceKind)
	}
	if len(body.Results) != 2 {
		t.Fatalf("results = %d, want 2", len(body.Results))
	}
	raw := response.Body.String()
	for _, forbidden := range []string{`"reasons"`, `"input_refs"`, `"attempt_no"`, `"risk_decision"`} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("response leaked forbidden field %s: %s", forbidden, raw)
		}
	}
	if port.runRef != "run-1" || port.calls != 1 {
		t.Fatalf("port call = %+v", port)
	}
}

func TestResultReadHTTPHandlerDeniedAndUnavailable(t *testing.T) {
	t.Run("access denied maps to 403", func(t *testing.T) {
		handler := &researchflowHTTPHandler{
			resultRead: &resultReadHTTPPortStub{err: researchflowapplication.ErrRunAccessDenied},
		}
		request := httptest.NewRequest(http.MethodGet, "/api/v1/researchflow/runs/run-1/results", nil)
		request.SetPathValue("run_ref", "run-1")
		response := httptest.NewRecorder()
		handler.getResults(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", response.Code)
		}
	})

	t.Run("unconfigured read port maps to 404", func(t *testing.T) {
		handler := &researchflowHTTPHandler{}
		request := httptest.NewRequest(http.MethodGet, "/api/v1/researchflow/runs/run-1/results", nil)
		request.SetPathValue("run_ref", "run-1")
		response := httptest.NewRecorder()
		handler.getResults(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", response.Code)
		}
	})
}
