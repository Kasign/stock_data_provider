package app

import "context"

// EvidenceQueryFacade is the only cross-module Research read interface.
// Construction requires both authorized single-item and page facades.
type EvidenceQueryFacade struct {
	get  *GetEvidence
	list *ListEvidence
}

func NewEvidenceQueryFacade(get *GetEvidence, list *ListEvidence) (*EvidenceQueryFacade, error) {
	if get == nil || list == nil {
		return nil, ErrInvalidEvidenceQuery
	}
	return &EvidenceQueryFacade{get: get, list: list}, nil
}

func (f *EvidenceQueryFacade) GetEvidence(ctx context.Context, request GetEvidenceRequest) (ResearchEvidenceRecord, error) {
	if f == nil || f.get == nil {
		return ResearchEvidenceRecord{}, ErrInvalidEvidenceQuery
	}
	return f.get.Execute(ctx, request)
}

func (f *EvidenceQueryFacade) ListEvidence(ctx context.Context, request ListEvidenceRequest) (ResearchEvidencePage, error) {
	if f == nil || f.list == nil {
		return ResearchEvidencePage{}, ErrInvalidEvidenceQuery
	}
	return f.list.Execute(ctx, request)
}

var _ ResearchEvidenceQuery = (*EvidenceQueryFacade)(nil)
