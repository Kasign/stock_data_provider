package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

const quantAssociationTestRunRef = "run:researchflow:001"

const quantAssociationSMA20ResultID = "quant_result:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

// quantAssociationUnrelatedResultID 与生产 fixture 的 result_id 长度/格式一致但内容不同。
const quantAssociationUnrelatedResultID = "quant_result:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

const quantAssociationSMA20Payload = `{"schema_version":"1.0.0","result_id":"quant_result:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","snapshot_ref":"data_snapshot:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","metric_code":"sma","value":"43.1200","unit":"CNY/share","period":"20d","metric_status":"available","sample_count":20,"trade_date":"2026-10-30","system_as_of":"2026-11-20T08:00:00Z","quality_status":"valid","result_status":"completed"}`

const quantAssociationSMA20Quality = `{"schema_version":"1.0.0","quality_status":"valid","calendar_quality_status":"valid","expected_trading_days":20,"observed_bars":20,"minimum_required_bars":20,"coverage_sufficient":true}`

type quantAssociationRunReaderStub struct {
	result researchflowapplication.CreateRunResult
	err    error
	runRef string
	calls  int
}

func (stub *quantAssociationRunReaderStub) GetRun(_ context.Context, runRef string) (researchflowapplication.CreateRunResult, error) {
	stub.calls++
	stub.runRef = runRef
	return stub.result, stub.err
}

type quantAssociationOutputReaderStub struct {
	resources    map[string]researchflowapplication.StageOutputResource
	errs         map[string]error
	expectations []researchflowapplication.StageOutputExpectation
	runRefs      []string
}

func (stub *quantAssociationOutputReaderStub) Read(
	_ context.Context,
	runRef string,
	expectation researchflowapplication.StageOutputExpectation,
) (researchflowapplication.StageOutputResource, error) {
	stub.runRefs = append(stub.runRefs, runRef)
	stub.expectations = append(stub.expectations, expectation)
	if err, failed := stub.errs[expectation.StageCode]; failed {
		return researchflowapplication.StageOutputResource{}, err
	}
	resource, present := stub.resources[expectation.StageCode]
	if !present {
		return researchflowapplication.StageOutputResource{}, researchflowapplication.ErrStageOutputUnavailable
	}
	return resource, nil
}

func quantAssociationStageStage(runRef, code string, status researchflowdomain.StageStatus) researchflowdomain.Stage {
	outputRef := "stage_output:sha256:" + strings.Repeat("1", 64)
	qualityRef := "stage_output:sha256:" + strings.Repeat("2", 64)
	return researchflowdomain.Stage{
		Ref: "stage:" + code, RunRef: runRef, Code: code, Status: status, AttemptNo: 1,
		OutputRef: &outputRef, QualitySummaryRef: &qualityRef,
	}
}

func quantAssociationRun(runRef string, stages ...researchflowdomain.Stage) researchflowapplication.CreateRunResult {
	return researchflowapplication.CreateRunResult{
		Run:    researchflowdomain.Run{Ref: runRef},
		Stages: stages,
	}
}

func quantFactorAssociationPayload(t *testing.T) json.RawMessage {
	t.Helper()
	fixture := validQuantFactorStageAdapterStub()
	payload, err := json.Marshal(fixture.output)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestQuantResultAssociationVerifierMatchesSMA20AndFactorOutput(t *testing.T) {
	for _, test := range []struct {
		name             string
		stageCode        string
		outputType       string
		schemaVersion    string
		payloadSchemaRef string
		qualitySchemaRef string
		payload          []byte
		quality          []byte
		resultID         string
	}{
		{
			name: "sma20", stageCode: quantstage.SMA20StageCode,
			outputType: quantstage.SMA20OutputType, schemaVersion: quantstage.SMA20SchemaVersion,
			payloadSchemaRef: quantSMA20PayloadSchemaRef, qualitySchemaRef: quantSMA20QualitySchemaRef,
			payload: []byte(quantAssociationSMA20Payload), quality: []byte(quantAssociationSMA20Quality),
			resultID: quantAssociationSMA20ResultID,
		},
		{
			name: "factor", stageCode: quantstage.FactorStageCode,
			outputType: quantstage.FactorOutputType, schemaVersion: quantstage.FactorSchemaVersion,
			payloadSchemaRef: quantFactorPayloadSchemaRef, qualitySchemaRef: quantFactorQualitySchemaRef,
			payload:  quantFactorAssociationPayload(t),
			quality:  mustQuantFactorAssociationQuality(t),
			resultID: quantFactorTestResultID,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runs := &quantAssociationRunReaderStub{result: quantAssociationRun(
				quantAssociationTestRunRef,
				quantAssociationStageStage(quantAssociationTestRunRef, test.stageCode, researchflowdomain.StageSucceeded),
			)}
			outputs := &quantAssociationOutputReaderStub{resources: map[string]researchflowapplication.StageOutputResource{
				test.stageCode: {Content: test.payload},
			}}
			verifier := quantResultAssociationVerifier{runs: runs, outputs: outputs}
			if err := verifier.VerifyResultAssociation(context.Background(), quantAssociationTestRunRef, test.resultID); err != nil {
				t.Fatalf("VerifyResultAssociation() error = %v", err)
			}
			if runs.calls != 1 || runs.runRef != quantAssociationTestRunRef || len(outputs.runRefs) != 1 ||
				outputs.runRefs[0] != quantAssociationTestRunRef {
				t.Fatalf("association verifier did not bind the trusted Run reference: runs=%+v outputs=%+v", runs, outputs)
			}
			expectation := outputs.expectations[0]
			if expectation.StageCode != test.stageCode || expectation.OutputType != test.outputType ||
				expectation.SchemaVersion != test.schemaVersion ||
				expectation.SchemaRef != test.payloadSchemaRef ||
				expectation.QualitySummarySchemaRef != test.qualitySchemaRef ||
				expectation.PayloadValidator == nil || expectation.QualitySummaryValidator == nil {
				t.Fatalf("association reader did not pin the complete %s output contract: %+v", test.name, expectation)
			}
			if err := expectation.PayloadValidator(test.payload); err != nil {
				t.Fatalf("%s Payload validator rejected the valid association fixture: %v", test.name, err)
			}
			if err := expectation.QualitySummaryValidator(test.quality); err != nil {
				t.Fatalf("%s QualitySummary validator rejected the valid association fixture: %v", test.name, err)
			}
		})
	}
}

func mustQuantFactorAssociationQuality(t *testing.T) []byte {
	t.Helper()
	quality, err := json.Marshal(validQuantFactorStageAdapterStub().quality)
	if err != nil {
		t.Fatal(err)
	}
	return quality
}

func TestQuantResultAssociationVerifierDoesNotReadUnrelatedStages(t *testing.T) {
	factorPayload := quantFactorAssociationPayload(t)
	for _, test := range []struct {
		name      string
		resultID  string
		stages    []researchflowdomain.Stage
		resources map[string]researchflowapplication.StageOutputResource
		wantReads int
	}{
		{
			name: "different result id", resultID: quantAssociationUnrelatedResultID,
			stages: []researchflowdomain.Stage{quantAssociationStageStage(
				quantAssociationTestRunRef, quantstage.FactorStageCode, researchflowdomain.StageSucceeded)},
			resources: map[string]researchflowapplication.StageOutputResource{
				quantstage.FactorStageCode: {Content: factorPayload},
			},
			wantReads: 1,
		},
		{
			name: "run has no Quant stage", resultID: quantAssociationSMA20ResultID,
			stages: []researchflowdomain.Stage{{
				Ref: "stage:news.search", RunRef: quantAssociationTestRunRef,
				Code: "news.search", Status: researchflowdomain.StageSucceeded, AttemptNo: 1,
			}},
			wantReads: 0,
		},
		{
			name: "Quant stage did not succeed", resultID: quantAssociationSMA20ResultID,
			stages: []researchflowdomain.Stage{quantAssociationStageStage(
				quantAssociationTestRunRef, quantstage.SMA20StageCode, researchflowdomain.StageFailed)},
			wantReads: 0,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			runs := &quantAssociationRunReaderStub{result: quantAssociationRun(quantAssociationTestRunRef, test.stages...)}
			outputs := &quantAssociationOutputReaderStub{resources: test.resources}
			verifier := quantResultAssociationVerifier{runs: runs, outputs: outputs}
			err := verifier.VerifyResultAssociation(context.Background(), quantAssociationTestRunRef, test.resultID)
			if !errors.Is(err, quantapplication.ErrResultAssociationNotFound) {
				t.Fatalf("VerifyResultAssociation() error = %v, want ErrResultAssociationNotFound", err)
			}
			if len(outputs.expectations) != test.wantReads {
				t.Fatalf("association verifier read %d Stage Outputs, want %d: %+v", len(outputs.expectations), test.wantReads, outputs.expectations)
			}
		})
	}
}

func TestQuantResultAssociationVerifierFailsClosedOnInconsistentEvidence(t *testing.T) {
	missingOutput := quantAssociationStageStage(
		quantAssociationTestRunRef, quantstage.FactorStageCode, researchflowdomain.StageSucceeded)
	missingOutput.OutputRef = nil
	missingQuality := quantAssociationStageStage(
		quantAssociationTestRunRef, quantstage.SMA20StageCode, researchflowdomain.StageSucceeded)
	missingQuality.QualitySummaryRef = nil
	foreignRun := quantAssociationStageStage(
		"run:researchflow:other", quantstage.FactorStageCode, researchflowdomain.StageSucceeded)

	for _, test := range []struct {
		name string
		run  researchflowapplication.CreateRunResult
	}{
		{name: "succeeded stage without output ref", run: quantAssociationRun(quantAssociationTestRunRef, missingOutput)},
		{name: "succeeded stage without quality ref", run: quantAssociationRun(quantAssociationTestRunRef, missingQuality)},
		{name: "duplicate stage code", run: quantAssociationRun(quantAssociationTestRunRef,
			quantAssociationStageStage(quantAssociationTestRunRef, quantstage.FactorStageCode, researchflowdomain.StageSucceeded),
			quantAssociationStageStage(quantAssociationTestRunRef, quantstage.FactorStageCode, researchflowdomain.StageDegraded),
		)},
		{name: "stage belongs to another run", run: quantAssociationRun(quantAssociationTestRunRef, foreignRun)},
		{name: "run aggregate mismatch", run: researchflowapplication.CreateRunResult{
			Run:    researchflowdomain.Run{Ref: "run:researchflow:rewritten"},
			Stages: []researchflowdomain.Stage{missingOutput},
		}},
		{name: "run without stages", run: quantAssociationRun(quantAssociationTestRunRef)},
	} {
		t.Run(test.name, func(t *testing.T) {
			verifier := quantResultAssociationVerifier{
				runs:    &quantAssociationRunReaderStub{result: test.run},
				outputs: &quantAssociationOutputReaderStub{},
			}
			err := verifier.VerifyResultAssociation(context.Background(), quantAssociationTestRunRef, quantAssociationSMA20ResultID)
			if !errors.Is(err, researchflowapplication.ErrStageOutputIntegrity) {
				t.Fatalf("VerifyResultAssociation() error = %v, want ErrStageOutputIntegrity", err)
			}
		})
	}
}

func TestQuantResultAssociationVerifierPropagatesReadFailures(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "stage output unavailable", err: researchflowapplication.ErrStageOutputUnavailable},
		{name: "stage output integrity", err: researchflowapplication.ErrStageOutputIntegrity},
		{name: "storage failure", err: errors.New("stage output storage is unreachable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			verifier := quantResultAssociationVerifier{
				runs: &quantAssociationRunReaderStub{result: quantAssociationRun(quantAssociationTestRunRef,
					quantAssociationStageStage(quantAssociationTestRunRef, quantstage.FactorStageCode, researchflowdomain.StageSucceeded))},
				outputs: &quantAssociationOutputReaderStub{errs: map[string]error{quantstage.FactorStageCode: test.err}},
			}
			err := verifier.VerifyResultAssociation(context.Background(), quantAssociationTestRunRef, quantAssociationSMA20ResultID)
			if err == nil {
				t.Fatal("VerifyResultAssociation() accepted a failed Stage Output read")
			}
			if errors.Is(err, quantapplication.ErrResultAssociationNotFound) {
				t.Fatalf("Stage Output read failure was downgraded to a missing association: %v", err)
			}
			if !errors.Is(err, test.err) {
				t.Fatalf("VerifyResultAssociation() error = %v, want it to wrap %v", err, test.err)
			}
		})
	}
}

func TestQuantResultAssociationVerifierRejectsInvalidArguments(t *testing.T) {
	verifier := quantResultAssociationVerifier{}
	if err := verifier.VerifyResultAssociation(context.Background(), quantAssociationTestRunRef, quantAssociationSMA20ResultID); !errors.Is(err, quantapplication.ErrAuthorizationUnavailable) {
		t.Fatalf("nil dependencies error = %v, want ErrAuthorizationUnavailable", err)
	}
	usable := quantResultAssociationVerifier{
		runs:    &quantAssociationRunReaderStub{result: quantAssociationRun(quantAssociationTestRunRef)},
		outputs: &quantAssociationOutputReaderStub{},
	}
	for _, test := range []struct{ runRef, resultID string }{
		{runRef: "", resultID: quantAssociationSMA20ResultID},
		{runRef: " " + quantAssociationTestRunRef, resultID: quantAssociationSMA20ResultID},
		{runRef: quantAssociationTestRunRef, resultID: ""},
		{runRef: quantAssociationTestRunRef, resultID: quantAssociationSMA20ResultID + " "},
	} {
		if err := usable.VerifyResultAssociation(context.Background(), test.runRef, test.resultID); !errors.Is(err, quantapplication.ErrAuthorizationUnavailable) {
			t.Fatalf("VerifyResultAssociation(%q, %q) error = %v, want ErrAuthorizationUnavailable", test.runRef, test.resultID, err)
		}
	}
	// GetRun 失败必须上抛，不能当作无关联。
	failing := quantResultAssociationVerifier{
		runs:    &quantAssociationRunReaderStub{err: errors.New("run storage is unreachable")},
		outputs: &quantAssociationOutputReaderStub{},
	}
	if err := failing.VerifyResultAssociation(context.Background(), quantAssociationTestRunRef, quantAssociationSMA20ResultID); err == nil ||
		errors.Is(err, quantapplication.ErrResultAssociationNotFound) {
		t.Fatalf("Run read failure error = %v, want a propagated failure", err)
	}
}

func TestQuantStageAssociationSpecsCoverEverySupportedQuantStage(t *testing.T) {
	seen := map[string]struct{}{}
	for _, spec := range quantStageAssociationSpecs {
		if _, duplicate := seen[spec.StageCode]; duplicate {
			t.Fatalf("duplicate Quant Stage association spec for %q", spec.StageCode)
		}
		seen[spec.StageCode] = struct{}{}
		if strings.TrimSpace(spec.PayloadSchemaRef) == "" || strings.TrimSpace(spec.QualitySummarySchemaRef) == "" ||
			spec.PayloadValidator == nil || spec.QualitySummaryValidator == nil || spec.ResultID == nil {
			t.Fatalf("Quant Stage association spec %q is incomplete: %+v", spec.StageCode, spec)
		}
	}
	for _, stageCode := range []string{quantstage.SMA20StageCode, quantstage.FactorStageCode} {
		if _, supported := quantStageAssociationSpecFor(stageCode); !supported {
			t.Fatalf("Quant Stage %q is missing from the association spec table", stageCode)
		}
	}
	if _, supported := quantStageAssociationSpecFor("news.search"); supported {
		t.Fatal("association spec table accepted an unregistered Stage code")
	}
	sma20, _ := quantStageAssociationSpecFor(quantstage.SMA20StageCode)
	if sma20.PayloadSchemaRef != quantSMA20PayloadSchemaRef || sma20.QualitySummarySchemaRef != quantSMA20QualitySchemaRef {
		t.Fatalf("SMA20 association spec drifted from its frozen schema refs: %+v", sma20)
	}
	factor, _ := quantStageAssociationSpecFor(quantstage.FactorStageCode)
	if factor.PayloadSchemaRef != quantFactorPayloadSchemaRef || factor.QualitySummarySchemaRef != quantFactorQualitySchemaRef {
		t.Fatalf("Factor association spec drifted from its frozen schema refs: %+v", factor)
	}
}
