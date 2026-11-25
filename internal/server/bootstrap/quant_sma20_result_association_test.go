package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
)

type quantSMA20StageOutputReaderStub struct {
	resource    researchflowapplication.StageOutputResource
	expectation researchflowapplication.StageOutputExpectation
	runRef      string
	err         error
	calls       int
}

func (stub *quantSMA20StageOutputReaderStub) Read(
	_ context.Context,
	runRef string,
	expectation researchflowapplication.StageOutputExpectation,
) (researchflowapplication.StageOutputResource, error) {
	stub.calls++
	stub.runRef = runRef
	stub.expectation = expectation
	return stub.resource, stub.err
}

func TestQuantSMA20ResultAssociationVerifierUsesFrozenOutputContract(t *testing.T) {
	resource := researchflowapplication.StageOutputResource{
		Content: json.RawMessage(`{"schema_version":"1.0.0","result_id":"quant_result:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","snapshot_ref":"data_snapshot:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","metric_code":"sma","value":"43.1200","unit":"CNY/share","period":"20d","metric_status":"available","sample_count":20,"trade_date":"2026-10-30","system_as_of":"2026-11-20T08:00:00Z","quality_status":"valid","result_status":"completed"}`),
	}
	reader := &quantSMA20StageOutputReaderStub{resource: resource}
	verifier := quantSMA20ResultAssociationVerifier{reader: reader}
	if err := verifier.VerifyResultAssociation(context.Background(), "run:researchflow:001", "quant_result:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); err != nil {
		t.Fatalf("VerifyResultAssociation() error = %v", err)
	}
	if reader.calls != 1 || reader.runRef != "run:researchflow:001" ||
		reader.expectation.StageCode != quantstage.SMA20StageCode ||
		reader.expectation.OutputType != quantstage.SMA20OutputType ||
		reader.expectation.SchemaVersion != quantstage.SMA20SchemaVersion ||
		reader.expectation.SchemaRef != quantSMA20PayloadSchemaRef ||
		reader.expectation.QualitySummarySchemaRef != quantSMA20QualitySchemaRef ||
		reader.expectation.PayloadValidator == nil || reader.expectation.QualitySummaryValidator == nil {
		t.Fatalf("association reader did not pin the complete SMA20 output contract: %+v", reader)
	}
	if err := reader.expectation.PayloadValidator(resource.Content); err != nil {
		t.Fatalf("SMA20 Payload validator rejected valid association fixture: %v", err)
	}
	quality := json.RawMessage(`{"schema_version":"1.0.0","quality_status":"valid","calendar_quality_status":"valid","expected_trading_days":20,"observed_bars":20,"minimum_required_bars":20,"coverage_sufficient":true}`)
	if err := reader.expectation.QualitySummaryValidator(quality); err != nil {
		t.Fatalf("SMA20 QualitySummary validator rejected valid association fixture: %v", err)
	}
}

func TestQuantSMA20ResultAssociationVerifierRejectsMissingOrMismatchedOutput(t *testing.T) {
	for _, test := range []struct {
		name     string
		resource researchflowapplication.StageOutputResource
		err      error
		want     error
	}{
		{name: "missing Stage output", err: researchflowapplication.ErrStageOutputUnavailable, want: quantapplication.ErrResultAssociationNotFound},
		{
			name: "different result id",
			resource: researchflowapplication.StageOutputResource{
				Content: json.RawMessage(`{"result_id":"quant_result:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`),
			},
			want: quantapplication.ErrResultAssociationNotFound,
		},
		{name: "integrity failure", err: researchflowapplication.ErrStageOutputIntegrity, want: quantapplication.ErrAuthorizationUnavailable},
	} {
		t.Run(test.name, func(t *testing.T) {
			reader := &quantSMA20StageOutputReaderStub{resource: test.resource, err: test.err}
			verifier := quantSMA20ResultAssociationVerifier{reader: reader}
			err := verifier.VerifyResultAssociation(context.Background(), "run:researchflow:001", "quant_result:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			if !errors.Is(err, test.want) {
				t.Fatalf("VerifyResultAssociation() error = %v, want %v", err, test.want)
			}
		})
	}
}
