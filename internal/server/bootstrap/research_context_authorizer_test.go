package bootstrap

import (
	"context"
	"errors"
	"testing"
	"time"

	researchapplication "example.com/stock-analysis-flow/internal/research/application"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

type contextEvidenceRepositoryFake struct {
	record researchevidence.Evidence
	err    error
	get    int
}

func (f *contextEvidenceRepositoryFake) Append(context.Context, researchevidence.Evidence) error {
	return nil
}

func (f *contextEvidenceRepositoryFake) Get(context.Context, string) (researchevidence.Evidence, error) {
	f.get++
	return f.record, f.err
}

type contextEvidenceAuthorizerFake struct {
	calls  int
	err    error
	caller string
}

func (f *contextEvidenceAuthorizerFake) AuthorizeEvidence(_ context.Context, caller string, _ researchevidence.PurposeCode, _ researchevidence.Evidence) error {
	f.calls++
	f.caller = caller
	return f.err
}

func validAuthorizedResearchContext(systemAsOf time.Time) researchcontext.Context {
	return researchcontext.Context{
		SchemaVersion:  researchcontext.SchemaVersion,
		ContextID:      "ctx-1",
		ContextVersion: 1,
		SystemAsOf:     systemAsOf,
		SectionRefs: []researchcontext.SectionRef{{
			SectionID:      "news",
			SectionVersion: "1.0.0",
			SourceOwner:    researchcontext.SourceOwnerNews,
			EvidenceRefs:   []string{"ev-1"},
			Included:       true,
			SystemAsOf:     systemAsOf,
		}},
		IncludedSections: []string{"news"},
	}
}

func TestResearchflowContextAuthorizerRechecksIncludedEvidence(t *testing.T) {
	now := time.Now().UTC()
	repository := &contextEvidenceRepositoryFake{record: researchevidence.Evidence{EvidenceID: "ev-1", SystemAsOf: now}}
	authorizer := &contextEvidenceAuthorizerFake{}
	subject := &researchflowContextAuthorizer{
		callerRef: researchflowCallerRef, evidence: repository, evidenceAuthorizer: authorizer,
	}
	if err := subject.AuthorizeContext(context.Background(), researchflowCallerRef, researchevidence.PurposeResearchAnalysis, validAuthorizedResearchContext(now)); err != nil {
		t.Fatalf("AuthorizeContext() error = %v", err)
	}
	if repository.get != 1 || authorizer.calls != 1 || authorizer.caller != researchflowCallerRef {
		t.Fatalf("evidence authorization was not rechecked: get=%d calls=%d caller=%s", repository.get, authorizer.calls, authorizer.caller)
	}
}

func TestResearchflowContextAuthorizerRejectsUntrustedCaller(t *testing.T) {
	now := time.Now().UTC()
	repository := &contextEvidenceRepositoryFake{record: researchevidence.Evidence{EvidenceID: "ev-1", SystemAsOf: now}}
	authorizer := &contextEvidenceAuthorizerFake{}
	subject := &researchflowContextAuthorizer{
		callerRef: researchflowCallerRef, evidence: repository, evidenceAuthorizer: authorizer,
	}
	err := subject.AuthorizeContext(context.Background(), "untrusted", researchevidence.PurposeResearchAnalysis, validAuthorizedResearchContext(now))
	if err == nil || !errors.Is(err, errResearchflowContextAccessDenied) {
		t.Fatalf("untrusted caller error = %v, want access denied", err)
	}
	if authorizer.calls != 0 {
		t.Fatal("untrusted caller reached Evidence authorization")
	}
}

func TestResearchflowContextAuthorizerRejectsNonNewsSection(t *testing.T) {
	now := time.Now().UTC()
	value := validAuthorizedResearchContext(now)
	value.SectionRefs[0].SourceOwner = researchcontext.SourceOwnerQuant
	subject := &researchflowContextAuthorizer{
		callerRef:          researchflowCallerRef,
		evidence:           &contextEvidenceRepositoryFake{record: researchevidence.Evidence{EvidenceID: "ev-1", SystemAsOf: now}},
		evidenceAuthorizer: &contextEvidenceAuthorizerFake{},
	}
	if err := subject.AuthorizeContext(context.Background(), researchflowCallerRef, researchevidence.PurposeResearchAnalysis, value); err == nil {
		t.Fatal("non-News section must fail closed")
	}
}

func TestResearchflowContextAuthorizerRejectsEvidenceAuthorizationFailure(t *testing.T) {
	now := time.Now().UTC()
	subject := &researchflowContextAuthorizer{
		callerRef: researchflowCallerRef,
		evidence:  &contextEvidenceRepositoryFake{record: researchevidence.Evidence{EvidenceID: "ev-1", SystemAsOf: now}},
		evidenceAuthorizer: &contextEvidenceAuthorizerFake{
			err: researchapplication.NewAuthorizationError(researchapplication.AuthorizationDenied, errors.New("revoked")),
		},
	}
	if err := subject.AuthorizeContext(context.Background(), researchflowCallerRef, researchevidence.PurposeResearchAnalysis, validAuthorizedResearchContext(now)); err == nil {
		t.Fatal("revoked Evidence authorization must fail closed")
	}
}
