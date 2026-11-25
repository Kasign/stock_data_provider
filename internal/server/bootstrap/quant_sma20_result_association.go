package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
)

type quantSMA20StageOutputReader interface {
	Read(context.Context, string, researchflowapplication.StageOutputExpectation) (researchflowapplication.StageOutputResource, error)
}

// quantSMA20ResultAssociationVerifier adapts Researchflow's persisted Run/Stage/
// Attempt association into Quant's narrow result-association port.
type quantSMA20ResultAssociationVerifier struct {
	reader quantSMA20StageOutputReader
}

var _ quantapplication.ResultAssociationVerifier = quantSMA20ResultAssociationVerifier{}

func (verifier quantSMA20ResultAssociationVerifier) VerifyResultAssociation(
	ctx context.Context,
	runRef, resultID string,
) error {
	if verifier.reader == nil || ctx == nil || runRef == "" || resultID == "" {
		return quantapplication.ErrAuthorizationUnavailable
	}
	resource, err := verifier.reader.Read(ctx, runRef, researchflowapplication.StageOutputExpectation{
		StageCode:               quantstage.SMA20StageCode,
		OutputType:              quantstage.SMA20OutputType,
		SchemaVersion:           quantstage.SMA20SchemaVersion,
		SchemaRef:               quantSMA20PayloadSchemaRef,
		QualitySummarySchemaRef: quantSMA20QualitySchemaRef,
		PayloadValidator: func(value json.RawMessage) error {
			return quantstage.ValidateSMA20OutputJSON(value)
		},
		QualitySummaryValidator: func(value json.RawMessage) error {
			return quantstage.ValidateSMA20QualitySummaryJSON(value)
		},
	})
	if err != nil {
		if errors.Is(err, researchflowapplication.ErrStageOutputUnavailable) {
			return quantapplication.ErrResultAssociationNotFound
		}
		return fmt.Errorf("verify Researchflow quant.sma20 Stage Output association: %w", err)
	}
	var output quantstage.SMA20Output
	if err := json.Unmarshal(resource.Content, &output); err != nil {
		return fmt.Errorf("decode validated Quant SMA20 Stage Output: %w", err)
	}
	if output.ResultID != resultID {
		return quantapplication.ErrResultAssociationNotFound
	}
	return nil
}
