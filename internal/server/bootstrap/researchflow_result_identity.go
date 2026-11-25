package bootstrap

import (
	"context"
	"errors"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
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
