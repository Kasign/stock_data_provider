package application

import (
	"context"
	"errors"
	"testing"
	"time"

	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
	researcherrors "example.com/stock-analysis-flow/internal/research"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
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
		ResearchQuestion:"盈利能力变化是否持续", Scope:string(researchcontext.ScopeStock), SubjectRefs:[]string{"security:600000"},
		SystemAsOf:now, SectionRefs:[]ContextSectionRef{{
			SectionID:"financials",SectionVersion:"1",SourceOwner:string(researchcontext.SourceOwnerData),
			SourceRefs:[]string{"data:snapshot:1"},EvidenceRefs:[]string{},QualityStatus:string(researchcontext.QualityValid),
			Required:true,Included:true,AssembledAt:now,SystemAsOf:now,
		}}, QualitySummary:ContextQualitySummary{Status:string(researchcontext.QualityValid),ReasonCodes:[]string{}},
		IncludedSections:[]string{"financials"},OmittedSections:[]ContextOmittedSection{},BuilderVersion:"1.0.0",
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
	got,err:=service.AddEvidence(context.Background(),AddEvidenceCommand{CallerRef:"researchflow:worker-1",Draft:EvidenceDraft{EvidenceType:string(draft.EvidenceType),SourceOwner:string(draft.SourceOwner),SourceRef:draft.SourceRef,DerivedFromRefs:draft.DerivedFromRefs,SubjectRefs:draft.SubjectRefs,SystemObservedAt:draft.SystemObservedAt,SystemAsOf:draft.SystemAsOf,Stance:string(draft.Stance),QualityStatus:string(draft.QualityStatus),ContentLevel:string(draft.ContentLevel),AuthorizationRef:draft.AuthorizationRef,Limitations:draft.Limitations}})
	if err!=nil { t.Fatal(err) }
	if got.EvidenceID!="ev_test-1" || got.ContentFingerprint=="" || !repo.appended || auth.calls!=1 { t.Fatalf("Evidence application flow incomplete: %+v",got) }
	if _,err:=service.GetEvidence(context.Background(),EvidenceRef{EvidenceID:got.EvidenceID,CallerRef:"researchflow:worker-1"}); err!=nil { t.Fatalf("authorized Evidence read failed: %v",err) }
	auth.err=errors.New("revoked")
	if _,err:=service.GetEvidence(context.Background(),EvidenceRef{EvidenceID:got.EvidenceID,CallerRef:"researchflow:worker-1"}); err==nil { t.Fatal("revoked Evidence must not be returned") }
	repo.appended=false
	if _,err:=service.AddEvidence(context.Background(),AddEvidenceCommand{CallerRef:"researchflow:worker-1",Draft:EvidenceDraft{EvidenceType:string(draft.EvidenceType),SourceOwner:string(draft.SourceOwner),SourceRef:draft.SourceRef,DerivedFromRefs:draft.DerivedFromRefs,SubjectRefs:draft.SubjectRefs,SystemObservedAt:draft.SystemObservedAt,SystemAsOf:draft.SystemAsOf,Stance:string(draft.Stance),QualityStatus:string(draft.QualityStatus),ContentLevel:string(draft.ContentLevel),AuthorizationRef:draft.AuthorizationRef,Limitations:draft.Limitations}}); err==nil { t.Fatal("revoked Evidence authorization must be rejected") }
	if repo.appended { t.Fatal("unauthorized Evidence must not be persisted") }
}
func TestNewServiceRequiresSourceOwnerAuthorizationAdapter(t *testing.T) {
	_,err:=NewService(&contextRepoFake{},&evidenceRepoFake{},nil,nil,nil)
	if err==nil { t.Fatal("missing EvidenceAuthorizer must be rejected") }
}

type eventRepoFake struct { value researchevent.ResearchEvent; appended bool; lifecycleAppended bool; correlationID string; reasonCode *string; appendErr error; getErr error }
func (f *eventRepoFake) Append(_ context.Context, value researchevent.ResearchEvent) error { f.value=value; f.appended=true; return f.appendErr }
func (f *eventRepoFake) AppendWithLifecycle(_ context.Context, value researchevent.ResearchEvent, reasonCode *string, correlationID string) error {
	f.value=value; f.appended=true; f.lifecycleAppended=true; f.reasonCode=reasonCode; f.correlationID=correlationID; return f.appendErr
}
func (f *eventRepoFake) GetVersion(_ context.Context, id string, version int) (researchevent.ResearchEvent,error) {
	if f.getErr != nil { return researchevent.ResearchEvent{}, f.getErr }
	if f.value.EventID!=id || f.value.EventVersion!=version { return researchevent.ResearchEvent{}, errors.New("missing") }
	return f.value,nil
}

func TestCreateAndGetEventUseApplicationDTOAndServerIdentity(t *testing.T) {
	now:=time.Date(2026,11,9,1,0,0,0,time.UTC)
	repo:=&eventRepoFake{}
	service,err:=NewServiceWithEvents(&contextRepoFake{},&evidenceRepoFake{},repo,&authorizerFake{},func()time.Time{return now},&fixedIDs{})
	if err!=nil { t.Fatal(err) }
	command:=CreateEventCommand{
		EventType:"earnings_guidance",Title:"公司调整年度业绩指引",Status:string(researchevent.StatusDetected),
		SubjectRefs:[]string{"security:600000"},EvidenceRefs:[]string{"ev_source_1"},
		SystemObservedAt:now,SystemAsOf:now,ImpactHypothesisRefs:[]string{},
		InvalidationConditions:[]string{"后续公告撤回该指引"},Uncertainty:[]string{},
		SourceTimes:[]EventSourceTime{{EvidenceRef:"ev_source_1",TimeType:string(researchevent.TimeSystemObservedAt),Value:now,AssertedBy:"news:observation"}},
	}
	createCtx, err := WithCorrelationID(context.Background(), "corr_create_parent")
	if err != nil { t.Fatal(err) }
	got,err:=service.CreateEvent(createCtx,command)
	if err!=nil { t.Fatal(err) }
	if got.EventID!="evt_test-1" || got.EventVersion!=1 || !repo.appended || !repo.lifecycleAppended { t.Fatalf("Event was not created with server identity and lifecycle Outbox: %+v",got) }
	if repo.correlationID != "corr_create_parent" || repo.reasonCode != nil { t.Fatalf("initial lifecycle metadata mismatch: %+v", repo) }
	read,err:=service.GetEvent(context.Background(),EventRef{EventID:got.EventID,EventVersion:1})
	if err!=nil { t.Fatalf("GetEvent failed: %v",err) }
	if read.EventID!=got.EventID || read.Status!=command.Status { t.Fatalf("unexpected Event DTO: %+v",read) }
}

func TestUpdateEventUsesExpectedVersionAndDomainLifecycleRules(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	previous := researchevent.ResearchEvent{
		EventID: "evt_existing", EventVersion: 1, EventType: "earnings_guidance",
		Title: "公司调整年度业绩指引", Status: researchevent.StatusDetected,
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{"ev_source_1"},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{"公告撤回"},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{{
			EvidenceRef: "ev_source_1", TimeType: researchevent.TimeSystemObservedAt,
			Value: now, AssertedBy: "news:observation",
		}},
	}
	repo := &eventRepoFake{value: previous}
	service, err := NewServiceWithEvents(&contextRepoFake{}, &evidenceRepoFake{}, repo, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	revision := CreateEventCommand{
		EventType: previous.EventType, Title: "业绩指引进入汇总阶段",
		Status: string(researchevent.StatusConsolidating),
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{"ev_source_1"},
		SystemObservedAt: now, SystemAsOf: now.Add(time.Minute),
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{"公告撤回"},
		Uncertainty: []string{}, SourceTimes: []EventSourceTime{{
			EvidenceRef: "ev_source_1", TimeType: string(researchevent.TimeSystemObservedAt),
			Value: now, AssertedBy: "news:observation",
		}},
	}
	badReason := "reason is not a machine code"
	if _, err := service.UpdateEvent(context.Background(), UpdateEventCommand{
		EventID: previous.EventID, ExpectedVersion: 1, Revision: revision, ReasonCode: &badReason,
	}); err == nil {
		t.Fatal("reason_code containing spaces must be rejected")
	}
	if repo.appended { t.Fatal("invalid reason_code must not append an Event") }
	got, err := service.UpdateEvent(context.Background(), UpdateEventCommand{
		EventID: previous.EventID, ExpectedVersion: 1, Revision: revision,
	})
	if err != nil { t.Fatal(err) }
	if got.EventID != previous.EventID || got.EventVersion != 2 || got.Status != string(researchevent.StatusConsolidating) {
		t.Fatalf("unexpected Event revision: %+v", got)
	}
	if !repo.lifecycleAppended || repo.correlationID == "" || repo.reasonCode != nil {
		t.Fatalf("status change must use lifecycle Outbox append with a trusted correlation ID: %+v", repo)
	}
	if _, err := service.UpdateEvent(context.Background(), UpdateEventCommand{
		EventID: previous.EventID, ExpectedVersion: 1, Revision: revision,
	}); err == nil {
		t.Fatal("stale expected version must not be accepted")
	}
	if repo.value.EventVersion != 2 {
		t.Fatal("stale Event revision must not overwrite the latest version")
	}
	invalid := revision
	invalid.Status = string(researchevent.StatusDetected)
	if _, err := service.UpdateEvent(context.Background(), UpdateEventCommand{
		EventID: previous.EventID, ExpectedVersion: 2, Revision: invalid,
	}); err == nil {
		t.Fatal("backward lifecycle transition must be rejected")
	}
	if repo.value.EventVersion != 2 {
		t.Fatal("invalid lifecycle transition must not be persisted")
	}
}

func TestUpdateEventMapsMissingExpectedVersionToConflict(t *testing.T) {
	repo := &eventRepoFake{getErr: researcherrors.ErrNotFound}
	service, err := NewServiceWithEvents(&contextRepoFake{}, &evidenceRepoFake{}, repo, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	_, err = service.UpdateEvent(context.Background(), UpdateEventCommand{EventID: "evt_missing", ExpectedVersion: 1})
	if err == nil || !errors.Is(err, researcherrors.ErrConflict) {
		t.Fatalf("missing expected version error = %v, want conflict fault", err)
	}
	if repo.appended { t.Fatal("missing expected version must not append a new Event") }
}

func TestCreateEventRejectsInvalidEventBeforePersistence(t *testing.T) {
	now:=time.Date(2026,11,9,1,0,0,0,time.UTC)
	repo:=&eventRepoFake{}
	service,err:=NewServiceWithEvents(&contextRepoFake{},&evidenceRepoFake{},repo,&authorizerFake{},nil,&fixedIDs{})
	if err!=nil { t.Fatal(err) }
	_,err=service.CreateEvent(context.Background(),CreateEventCommand{EventType:"",Title:"",Status:"unknown",SystemObservedAt:now,SystemAsOf:now,SubjectRefs:[]string{},EvidenceRefs:[]string{},ImpactHypothesisRefs:[]string{},InvalidationConditions:[]string{},Uncertainty:[]string{},SourceTimes:[]EventSourceTime{}})
	if err==nil { t.Fatal("invalid Event must be rejected") }
	if repo.appended { t.Fatal("invalid Event must not be persisted") }
}

type claimRepoFake struct { value researchclaim.Claim; appended bool; appendErr error }
func (f *claimRepoFake) Append(_ context.Context, value researchclaim.Claim) error {
	if f.appendErr != nil { return f.appendErr }
	f.value=value; f.appended=true; return nil
}
func (f *claimRepoFake) GetVersion(_ context.Context, id string, version int) (researchclaim.Claim,error) {
	if f.value.ClaimID!=id || f.value.ClaimVersion!=version { return researchclaim.Claim{}, errors.New("missing") }
	return f.value,nil
}

func TestCreateAndGetHypothesisUseApplicationDTO(t *testing.T) {
	now:=time.Date(2026,11,9,1,0,0,0,time.UTC)
	repo:=&claimRepoFake{}
	service,err:=NewServiceWithClaims(&contextRepoFake{},&evidenceRepoFake{},repo,&authorizerFake{},nil,&fixedIDs{})
	if err!=nil { t.Fatal(err) }
	method:="比较公告前后盈利预测变化"
	observation:="公告后分析师预测上调"
	command:=CreateClaimCommand{
		Statement:"业绩指引上调将改善未来两个季度盈利预期",
		EpistemicType:string(researchclaim.EpistemicHypothesis),
		ScopeRefs:[]string{"security:600000"},SystemAsOf:now,
		SupportingEvidenceRefs:[]string{"ev_support_1"},ContradictingEvidenceRefs:[]string{},
		RequiredDataRefs:[]string{"data:forecast_revision"},ValidationMethod:&method,
		ExpectedObservation:&observation,InvalidationConditions:[]string{"后续预测持续下调"},
		Status:string(researchclaim.StatusProposed),Limitations:[]string{},
	}
	got,err:=service.CreateClaim(context.Background(),command)
	if err!=nil { t.Fatal(err) }
	if got.ClaimID!="clm_test-1" || got.ClaimVersion!=1 || !repo.appended { t.Fatalf("Claim was not created: %+v",got) }
	read,err:=service.GetClaim(context.Background(),ClaimRef{ClaimID:got.ClaimID,ClaimVersion:1})
	if err!=nil { t.Fatalf("GetClaim failed: %v",err) }
	if read.Statement!=command.Statement || read.EpistemicType!=command.EpistemicType { t.Fatalf("unexpected Claim DTO: %+v",read) }
}

func TestCreateHypothesisRequiresValidationContract(t *testing.T) {
	now:=time.Date(2026,11,9,1,0,0,0,time.UTC)
	repo:=&claimRepoFake{}
	service,err:=NewServiceWithClaims(&contextRepoFake{},&evidenceRepoFake{},repo,&authorizerFake{},nil,&fixedIDs{})
	if err!=nil { t.Fatal(err) }
	_,err=service.CreateClaim(context.Background(),CreateClaimCommand{
		Statement:"待验证假设",EpistemicType:string(researchclaim.EpistemicHypothesis),
		ScopeRefs:[]string{},SystemAsOf:now,SupportingEvidenceRefs:[]string{},
		ContradictingEvidenceRefs:[]string{},RequiredDataRefs:[]string{},
		InvalidationConditions:[]string{"条件失效"},Status:string(researchclaim.StatusProposed),Limitations:[]string{},
	})
	if err==nil { t.Fatal("Hypothesis without validation method/observation must be rejected") }
	if repo.appended { t.Fatal("invalid Hypothesis must not be persisted") }
}


func TestUpdateClaimEnforcesExpectedVersionAndDomainTransition(t *testing.T) {
	now := time.Date(2026, 11, 9, 2, 0, 0, 0, time.UTC)
	repo := &claimRepoFake{value: researchclaim.Claim{
		ClaimID: "clm_existing", ClaimVersion: 1, Statement: "待验证的盈利改善判断",
		EpistemicType: researchclaim.EpistemicHypothesis, ScopeRefs: []string{"security:600000"},
		SystemAsOf: now.Add(-time.Minute), SupportingEvidenceRefs: []string{},
		ContradictingEvidenceRefs: []string{}, RequiredDataRefs: []string{"data:forecast"},
		ValidationMethod: stringPointer("比较预测修订"), ExpectedObservation: stringPointer("预测上调"),
		InvalidationConditions: []string{"预测持续下调"}, Status: researchclaim.StatusProposed, Limitations: []string{},
	}}
	service, err := NewServiceWithClaims(&contextRepoFake{}, &evidenceRepoFake{}, repo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil { t.Fatal(err) }
	revision := CreateClaimCommand{
		Statement: "盈利改善判断进入验证阶段", EpistemicType: string(researchclaim.EpistemicHypothesis),
		ScopeRefs: []string{"security:600000"}, SystemAsOf: now,
		SupportingEvidenceRefs: []string{"ev_support_1"}, ContradictingEvidenceRefs: []string{},
		RequiredDataRefs: []string{"data:forecast"}, ValidationMethod: stringPointer("比较预测修订"),
		ExpectedObservation: stringPointer("预测上调"), InvalidationConditions: []string{"预测持续下调"},
		Status: string(researchclaim.StatusTesting), Limitations: []string{},
	}
	got, err := service.UpdateClaim(context.Background(), UpdateClaimCommand{ClaimID: "clm_existing", ExpectedVersion: 1, Revision: revision})
	if err != nil { t.Fatal(err) }
	if got.ClaimID != "clm_existing" || got.ClaimVersion != 2 || got.Status != string(researchclaim.StatusTesting) {
		t.Fatalf("unexpected Claim revision: %+v", got)
	}
	if _, err := service.UpdateClaim(context.Background(), UpdateClaimCommand{ClaimID: "clm_existing", ExpectedVersion: 1, Revision: revision}); err == nil {
		t.Fatal("stale expected version must not be accepted")
	}
	terminal := revision
	terminal.Status = string(researchclaim.StatusSupported)
	if _, err := service.UpdateClaim(context.Background(), UpdateClaimCommand{ClaimID: "clm_existing", ExpectedVersion: 2, Revision: terminal}); err == nil {
		t.Fatal("terminal Claim must not transition to another status")
	}
	if repo.value.ClaimVersion != 2 {
		t.Fatal("invalid terminal transition must not be persisted")
	}
}

func stringPointer(value string) *string { return &value }


type mismatchedContextRepo struct { contextRepoFake }
func (f *mismatchedContextRepo) GetVersion(context.Context, string, int64) (researchcontext.Context, error) { return f.value, nil }
type mismatchedEventRepo struct { eventRepoFake }
func (f *mismatchedEventRepo) GetVersion(context.Context, string, int) (researchevent.ResearchEvent, error) { return f.value, nil }
type mismatchedClaimRepo struct { claimRepoFake }
func (f *mismatchedClaimRepo) GetVersion(context.Context, string, int) (researchclaim.Claim, error) { return f.value, nil }

func TestApplicationRejectsRepositoryIdentityMismatch(t *testing.T) {
	now := time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC)
	contextRepo := &mismatchedContextRepo{contextRepoFake: contextRepoFake{value: researchcontext.Context{
		SchemaVersion: researchcontext.SchemaVersion, ContextID: "ctx_other", ContextVersion: 1,
		ResearchQuestion: "identity mismatch", Scope: researchcontext.ScopeStock,
		SubjectRefs: []string{"security:600000"}, SystemAsOf: now,
		SectionRefs: []researchcontext.SectionRef{}, QualitySummary: researchcontext.QualitySummary{
			Status: researchcontext.QualityValid, ReasonCodes: []string{},
		}, IncludedSections: []string{}, OmittedSections: []researchcontext.OmittedSection{},
		BuilderVersion: "test", CreatedAt: now,
	}}}
	service, err := NewService(contextRepo, &evidenceRepoFake{}, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	if _, err := service.GetContext(context.Background(), ContextRef{ContextID: "ctx_requested", ContextVersion: 1}); err == nil {
		t.Fatal("Context repository identity mismatch must be rejected")
	}

	eventRepo := &mismatchedEventRepo{eventRepoFake: eventRepoFake{value: researchevent.ResearchEvent{
		EventID: "evt_other", EventVersion: 1, EventType: "earnings_guidance", Title: "快照",
		Status: researchevent.StatusDetected, SubjectRefs: []string{"security:600000"},
		EvidenceRefs: []string{"ev_source_1"}, SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{}, Uncertainty: []string{},
		SourceTimes: []researchevent.EventSourceTime{{EvidenceRef: "ev_source_1", TimeType: researchevent.TimeSystemObservedAt, Value: now, AssertedBy: "news"}},
	}}}
	service, err = NewServiceWithEvents(&contextRepoFake{}, &evidenceRepoFake{}, eventRepo, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	if _, err := service.GetEvent(context.Background(), EventRef{EventID: "evt_requested", EventVersion: 1}); err == nil {
		t.Fatal("Event repository identity mismatch must be rejected")
	}

	claimRepo := &mismatchedClaimRepo{claimRepoFake: claimRepoFake{value: researchclaim.Claim{
		ClaimID: "clm_other", ClaimVersion: 1, Statement: "研究判断",
		EpistemicType: researchclaim.EpistemicReportedFact, ScopeRefs: []string{"security:600000"},
		SystemAsOf: now, SupportingEvidenceRefs: []string{"ev_source_1"},
		ContradictingEvidenceRefs: []string{}, RequiredDataRefs: []string{},
		Status: researchclaim.StatusProposed, Limitations: []string{},
	}}}
	service, err = NewServiceWithClaims(&contextRepoFake{}, &evidenceRepoFake{}, claimRepo, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	if _, err := service.GetClaim(context.Background(), ClaimRef{ClaimID: "clm_requested", ClaimVersion: 1}); err == nil {
		t.Fatal("Claim repository identity mismatch must be rejected")
	}
}



type mutatingEvidenceAuthorizer struct{}
func (mutatingEvidenceAuthorizer) AuthorizeEvidence(_ context.Context, _ string, _ researchevidence.PurposeCode, value researchevidence.Evidence) error {
	if value.SourceVersion != nil { *value.SourceVersion = "mutated" }
	if value.SourceLocator != nil { *value.SourceLocator = "mutated" }
	if value.ClaimRef != nil { *value.ClaimRef = "mutated" }
	if value.Confidence != nil { *value.Confidence = 0.99 }
	if value.ConfidenceMethod != nil { *value.ConfidenceMethod = "mutated" }
	if value.ConfidenceMethodVersion != nil { *value.ConfidenceMethodVersion = "mutated" }
	if value.PublishedAt != nil { *value.PublishedAt = value.PublishedAt.Add(time.Hour) }
	if value.AvailableAt != nil { *value.AvailableAt = value.AvailableAt.Add(time.Hour) }
	if value.ValidTimeStart != nil { *value.ValidTimeStart = value.ValidTimeStart.Add(time.Hour) }
	if value.ValidTimeEnd != nil { *value.ValidTimeEnd = value.ValidTimeEnd.Add(time.Hour) }
	if len(value.SubjectRefs) > 0 { value.SubjectRefs[0] = "mutated" }
	if len(value.DerivedFromRefs) > 0 { value.DerivedFromRefs[0] = "mutated" }
	if len(value.Limitations) > 0 { value.Limitations[0] = "mutated" }
	return nil
}

func TestEvidenceApplicationDeepCopiesDTOAndAdapterBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 9, 4, 0, 0, 0, time.UTC)
	sourceVersion, locator, claimRef := "v1", "paragraph:2", "claim:1"
	confidence, method, methodVersion := 0.7, "manual", "1"
	published, available, validStart, validEnd := now, now.Add(time.Minute), now.Add(-time.Hour), now.Add(time.Hour)
	draft := EvidenceDraft{
		EvidenceType: string(researchevidence.TypeFact), SourceOwner: string(researchevidence.SourceData),
		SourceRef: "data:fact:deep-copy", SourceVersion: &sourceVersion, SourceLocator: &locator,
		DerivedFromRefs: []string{"ev_parent"}, ClaimRef: &claimRef,
		SubjectRefs: []string{"security:600000"}, PublishedAt: &published, AvailableAt: &available,
		SystemObservedAt: now, ValidTimeStart: &validStart, ValidTimeEnd: &validEnd, SystemAsOf: now,
		Stance: string(researchevidence.StanceNeutral), Confidence: &confidence,
		ConfidenceMethod: &method, ConfidenceMethodVersion: &methodVersion,
		QualityStatus: string(researchevidence.QualityValid), ContentLevel: string(researchevidence.ContentMetadata),
		AuthorizationRef: "auth:deep-copy", Limitations: []string{"limited"},
	}
	repo := &evidenceRepoFake{}
	service, err := NewService(&contextRepoFake{}, repo, mutatingEvidenceAuthorizer{}, nil, &fixedIDs{})
	if err != nil { t.Fatal(err) }
	got, err := service.AddEvidence(context.Background(), AddEvidenceCommand{CallerRef: "researchflow:worker", Draft: draft})
	if err != nil { t.Fatal(err) }
	if *repo.value.SourceVersion != "v1" || *repo.value.SourceLocator != "paragraph:2" || *repo.value.ClaimRef != "claim:1" ||
		*repo.value.Confidence != 0.7 || *repo.value.ConfidenceMethod != "manual" || *repo.value.ConfidenceMethodVersion != "1" {
		t.Fatal("authorization adapter mutation leaked into the persisted Evidence")
	}
	if !repo.value.PublishedAt.Equal(published) || !repo.value.AvailableAt.Equal(available) ||
		!repo.value.ValidTimeStart.Equal(validStart) || !repo.value.ValidTimeEnd.Equal(validEnd) {
		t.Fatal("authorization adapter timestamp mutation leaked into the persisted Evidence")
	}
	if repo.value.SubjectRefs[0] != "security:600000" || repo.value.DerivedFromRefs[0] != "ev_parent" || repo.value.Limitations[0] != "limited" {
		t.Fatal("authorization adapter slice mutation leaked into the persisted Evidence")
	}
	// Mutating the caller's original pointers after the call must not mutate storage.
	sourceVersion, locator, claimRef, confidence, method, methodVersion = "caller", "caller", "caller", 0.1, "caller", "caller"
	published, available, validStart, validEnd = now.Add(2*time.Hour), now.Add(2*time.Hour), now.Add(2*time.Hour), now.Add(2*time.Hour)
	if *repo.value.SourceVersion != "v1" || *repo.value.Confidence != 0.7 || !repo.value.PublishedAt.Equal(now) {
		t.Fatal("caller-owned pointer mutation leaked into stored Evidence")
	}
	// Returned DTO pointers and slices must also be detached from repository state.
	*got.SourceVersion = "response"
	*got.Confidence = 0.2
	got.SubjectRefs[0] = "response"
	if *repo.value.SourceVersion != "v1" || *repo.value.Confidence != 0.7 || repo.value.SubjectRefs[0] != "security:600000" {
		t.Fatal("returned DTO mutation leaked into repository state")
	}
}


type mismatchedEvidenceRepo struct { evidenceRepoFake }
func (f *mismatchedEvidenceRepo) Get(context.Context, string) (researchevidence.Evidence, error) {
	value := f.value
	value.EvidenceID = "ev_other"
	return value, nil
}

func TestGetEvidenceRejectsRepositoryIdentityMismatch(t *testing.T) {
	now := time.Date(2026, 11, 9, 6, 0, 0, 0, time.UTC)
	value := researchevidence.Evidence{
		EvidenceID: "ev_other", EvidenceType: researchevidence.TypeFact,
		SourceOwner: researchevidence.SourceData, SourceRef: "data:fact:1",
		DerivedFromRefs: []string{}, SubjectRefs: []string{"security:600000"},
		SystemObservedAt: now, SystemAsOf: now, Stance: researchevidence.StanceNeutral,
		QualityStatus: researchevidence.QualityValid, ContentLevel: researchevidence.ContentMetadata,
		AuthorizationRef: "auth:1", Limitations: []string{},
	}
	fingerprint, err := value.ComputeContentFingerprint()
	if err != nil { t.Fatal(err) }
	value.ContentFingerprint = fingerprint
	repo := &mismatchedEvidenceRepo{evidenceRepoFake: evidenceRepoFake{value: value}}
	service, err := NewService(&contextRepoFake{}, repo, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	if _, err := service.GetEvidence(context.Background(), EvidenceRef{
		EvidenceID: "ev_requested", CallerRef: "researchflow:worker",
	}); err == nil {
		t.Fatal("Evidence repository identity mismatch must be rejected")
	}
}
