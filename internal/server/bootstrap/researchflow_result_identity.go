package bootstrap

import (
	"context"
	"errors"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantresult "example.com/stock-analysis-flow/internal/quant/result"
)

var errResearchflowWorkerPrincipalUnavailable = errors.New("Researchflow Worker identity is unavailable")

type researchflowWorkerPrincipalContextKey struct{}

// withResearchflowWorkerPrincipal marks only the trusted in-process Worker
// execution boundary. The marker cannot be supplied through HTTP input.
func withResearchflowWorkerPrincipal(ctx context.Context) context.Context {
	if ctx == nil {
		return nil
	}
	return context.WithValue(ctx, researchflowWorkerPrincipalContextKey{}, true)
}

type researchflowWorkerPrincipalResolver struct{}

var _ quantapplication.PrincipalResolver = researchflowWorkerPrincipalResolver{}

func (researchflowWorkerPrincipalResolver) Resolve(ctx context.Context) (quantapplication.Principal, error) {
	if ctx == nil || ctx.Value(researchflowWorkerPrincipalContextKey{}) != true {
		return quantapplication.Principal{}, errResearchflowWorkerPrincipalUnavailable
	}
	return quantapplication.Principal{SubjectRef: "service:researchflow"}, nil
}

type researchflowQuantResultReader struct {
	getResult *quantapplication.GetResult
}

// Read adds the internal Researchflow Worker identity only for this explicit
// downstream result-read operation; unrelated Stages do not inherit it.
func (reader *researchflowQuantResultReader) Read(
	ctx context.Context,
	runRef, resultID string,
) (quantresult.Envelope, error) {
	if reader == nil || reader.getResult == nil || ctx == nil {
		return quantresult.Envelope{}, errResearchflowWorkerPrincipalUnavailable
	}
	return reader.getResult.Execute(withResearchflowWorkerPrincipal(ctx), runRef, resultID)
}
