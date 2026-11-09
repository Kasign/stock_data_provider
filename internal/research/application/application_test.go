package application

import (
	"context"
	"errors"
	"testing"
	"time"

	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

type fixedIDs struct { next int }
func (f *fixedIDs) NewID(prefix string) (string,error) { f.next++; return prefix + "test-" + string(rune('0'+f.next)), nil }
type contextRepoFake struct { value researchcontext.Context; appendErr error }
func (f *contextRepoFake) Append(_ context.Context, value researchcontext.Context) error { f.value=value; return f.appendErr }
func (f *contextRepoFake) GetVersion(_ context.Context, id string, version int64) (researchcontext.Context,error) {
	if f.value.ContextID!=id || f.value.ContextVersion!=version { return researchcontext.Context{}, errors.New("missing") }
	return f.value,nil
}
type evidenceRepoFake struct { value researchevidence.Evidence; appended bool; appendErr error }
func (f *evidenceRepoFake) Append(_ context.Context, value researchevidence.Evidence) error { f.value=value; f.appended=true; return f.appendErr }
func (f *evidenceRepoFake) Get(_ context.Context, id string) (researchevidence.Evidence,error) {
	if f.value.EvidenceID!=id { return researchevidence.Evidence{}, errors.New("missing") }
	return f.value,nil
}
type authorizerFake struct { err error; calls int }
func (f *authorizerFake) AuthorizeEvidence(_ context.Context, caller string, purpose researchevidence.PurposeCode, value researchevidence.Evidence) error {
	f.calls++
	if caller=="" || purpose=="" || value.EvidenceID=="" { return errors.New("invalid authorization input") }
	return f.err
}

func TestCreateContextAssignsIdentityAndPersistsValidatedSnapshot(t *testing.T) {
	now:=time.Date(2026,11,9,1,0,0,0,time.UTC)
	repo:=&contextRepoFake{}
	service,err:=NewService(repo,&evidenceRepoFake{},&authorizerFake{},func()time.Time{return now},&fixedIDs{})
	if err!=nil { t.Fatal(err) }
	command:=CreateContextCommand{
		ResearchQuestion:"盈利能力变化是否持续", Scope:researchcontext.ScopeStock, SubjectRefs:[]string{"security:600000"},
		SystemAsOf:now, SectionRefs:[]researchcontext.SectionRef{{
			SectionID:"financials",SectionVersion:"1",SourceOwner:researchcontext.SourceOwnerData,
			SourceRefs:[]string{"data:snapshot:1"},EvidenceRefs:[]string{},QualityStatus:researchcontext.QualityValid,
			Required:true,Included:true,AssembledAt:now,SystemAsOf:now,
		}}, QualitySummary:researchcontext.QualitySummary{Status:researchcontext.QualityValid,ReasonCodes:[]string{}},
		IncludedSections:[]string{"financials"},OmittedSections:[]researchcontext.OmittedSection{},BuilderVersion:"1.0.0",
	}
	got,err:=service.CreateContext(context.Background(),command)
	if err!=nil { t.Fatal(err) }
	if got.ContextID!="ctx_test-1" || got.ContextVersion!=1 || !got.CreatedAt.Equal(now) { t.Fatalf("unexpected generated Context metadata: %+v",got) }
	if repo.value.ContextID!=got.ContextID { t.Fatal("Context was not persisted") }
}

func TestAddEvidenceGeneratesIdentityFingerprintAndRequiresCurrentAuthorization(t *testing.T) {
	now:=time.Date(2026,11,9,1,0,0,0,time.UTC)
	repo:=&evidenceRepoFake{}
	auth:=&authorizerFake{}
	service,err:=NewService(&contextRepoFake{},repo,auth,func()time.Time{return now},&fixedIDs{})
	if err!=nil { t.Fatal(err) }
	draft:=researchevidence.Evidence{
		EvidenceType:researchevidence.TypeFact,SourceOwner:researchevidence.SourceData,SourceRef:"data:fact:1",
		DerivedFromRefs:[]string{},SubjectRefs:[]string{"security:600000"},SystemObservedAt:now,SystemAsOf:now,
		Stance:researchevidence.StanceNeutral,QualityStatus:researchevidence.QualityValid,
		ContentLevel:researchevidence.ContentMetadata,AuthorizationRef:"auth:1",Limitations:[]string{},
	}
	got,err:=service.AddEvidence(context.Background(),AddEvidenceCommand{CallerRef:"researchflow:worker-1",Draft:draft})
	if err!=nil { t.Fatal(err) }
	if got.EvidenceID!="ev_test-1" || got.ContentFingerprint=="" || !repo.appended || auth.calls!=1 { t.Fatalf("Evidence application flow incomplete: %+v",got) }
	if _,err:=service.GetEvidence(context.Background(),EvidenceRef{EvidenceID:got.EvidenceID,CallerRef:"researchflow:worker-1"}); err!=nil { t.Fatalf("authorized Evidence read failed: %v",err) }
	auth.err=errors.New("revoked")
	if _,err:=service.GetEvidence(context.Background(),EvidenceRef{EvidenceID:got.EvidenceID,CallerRef:"researchflow:worker-1"}); err==nil { t.Fatal("revoked Evidence must not be returned") }
	repo.appended=false
	if _,err:=service.AddEvidence(context.Background(),AddEvidenceCommand{CallerRef:"researchflow:worker-1",Draft:draft}); err==nil { t.Fatal("revoked Evidence authorization must be rejected") }
	if repo.appended { t.Fatal("unauthorized Evidence must not be persisted") }
}
func TestNewServiceRequiresSourceOwnerAuthorizationAdapter(t *testing.T) {
	_,err:=NewService(&contextRepoFake{},&evidenceRepoFake{},nil,nil,nil)
	if err==nil { t.Fatal("missing EvidenceAuthorizer must be rejected") }
}
