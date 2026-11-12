package application

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	researcherrors "example.com/stock-analysis-flow/internal/research"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

type fixedIDs struct{ next int }

func (f *fixedIDs) NewID(prefix string) (string, error) {
	f.next++
	return prefix + "test-" + string(rune('0'+f.next)), nil
}

type contextRepoFake struct {
	value     researchcontext.Context
	appendErr error
}

func (f *contextRepoFake) Append(_ context.Context, value researchcontext.Context) error {
	f.value = value
	return f.appendErr
}
func (f *contextRepoFake) GetVersion(_ context.Context, id string, version int64) (researchcontext.Context, error) {
	if f.value.ContextID != id || f.value.ContextVersion != version {
		return researchcontext.Context{}, errors.New("missing")
	}
	return f.value, nil
}

type evidenceRepoFake struct {
	value     researchevidence.Evidence
	values    map[string]researchevidence.Evidence
	appended  bool
	appendErr error
}

func (f *evidenceRepoFake) Append(_ context.Context, value researchevidence.Evidence) error {
	f.value = value
	if f.values == nil {
		f.values = make(map[string]researchevidence.Evidence)
	}
	f.values[value.EvidenceID] = value
	f.appended = true
	return f.appendErr
}
func (f *evidenceRepoFake) Get(_ context.Context, id string) (researchevidence.Evidence, error) {
	if value, ok := f.values[id]; ok {
		return value, nil
	}
	if f.value.EvidenceID == id {
		return f.value, nil
	}
	return researchevidence.Evidence{}, researcherrors.ErrNotFound
}

func evidenceFixtureWithStance(t *testing.T, id string, systemAsOf time.Time, stance researchevidence.Stance) researchevidence.Evidence {
	t.Helper()
	value := evidenceFixture(t, id, systemAsOf)
	value.Stance = stance
	fingerprint, err := value.ComputeContentFingerprint()
	if err != nil {
		t.Fatalf("ComputeContentFingerprint() error = %v", err)
	}
	value.ContentFingerprint = fingerprint
	return value
}

func evidenceFixture(t *testing.T, id string, systemAsOf time.Time) researchevidence.Evidence {
	t.Helper()
	availableAt := systemAsOf.Add(-time.Hour)
	value := researchevidence.Evidence{
		EvidenceID: id, EvidenceType: researchevidence.TypeFact,
		SourceOwner: researchevidence.SourceData, SourceRef: "data:" + id,
		DerivedFromRefs: []string{}, SubjectRefs: []string{"security:600000"},
		AvailableAt: &availableAt, SystemObservedAt: systemAsOf.Add(-30 * time.Minute),
		SystemAsOf: systemAsOf, Stance: researchevidence.StanceNeutral,
		QualityStatus: researchevidence.QualityValid, ContentLevel: researchevidence.ContentMetadata,
		AuthorizationRef: "auth:" + id, Limitations: []string{},
	}
	fingerprint, err := value.ComputeContentFingerprint()
	if err != nil {
		t.Fatalf("ComputeContentFingerprint() error = %v", err)
	}
	value.ContentFingerprint = fingerprint
	return value
}

type authorizerFake struct {
	err   error
	calls int
}

type contextAuthorizerFake struct {
	err     error
	calls   int
	caller  string
	purpose researchevidence.PurposeCode
	value   researchcontext.Context
}

func (f *contextAuthorizerFake) AuthorizeContext(_ context.Context, caller string, purpose researchevidence.PurposeCode, value researchcontext.Context) error {
	f.calls++
	f.caller, f.purpose, f.value = caller, purpose, value
	return f.err
}

func (f *authorizerFake) AuthorizeEvidence(_ context.Context, caller string, purpose researchevidence.PurposeCode, value researchevidence.Evidence) error {
	f.calls++
	if caller == "" || purpose == "" || value.EvidenceID == "" {
		return errors.New("invalid authorization input")
	}
	return f.err
}

func TestCreateContextAssignsIdentityAndPersistsValidatedSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	repo := &contextRepoFake{}
	service, err := NewService(repo, &evidenceRepoFake{}, &authorizerFake{}, func() time.Time { return now }, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	command := CreateContextCommand{
		ResearchQuestion: "盈利能力变化是否持续", Scope: string(researchcontext.ScopeStock), SubjectRefs: []string{"security:600000"},
		SystemAsOf: now, SectionRefs: []ContextSectionRef{{
			SectionID: "financials", SectionVersion: "1", SourceOwner: string(researchcontext.SourceOwnerData),
			SourceRefs: []string{"data:snapshot:1"}, EvidenceRefs: []string{}, QualityStatus: string(researchcontext.QualityValid),
			Required: true, Included: true, AssembledAt: now, SystemAsOf: now,
		}}, QualitySummary: ContextQualitySummary{Status: string(researchcontext.QualityValid), ReasonCodes: []string{}},
		IncludedSections: []string{"financials"}, OmittedSections: []ContextOmittedSection{}, BuilderVersion: "1.0.0",
	}
	got, err := service.CreateContext(context.Background(), command)
	if err != nil {
		t.Fatalf("CreateContext() error = %v, cause = %v", err, errors.Unwrap(err))
	}
	if got.ContextID != "ctx_test-1" || got.ContextVersion != 1 || !got.CreatedAt.Equal(now) {
		t.Fatalf("unexpected generated Context metadata: %+v", got)
	}
	if repo.value.ContextID != got.ContextID {
		t.Fatal("Context was not persisted")
	}
}

func TestGetContextRequiresTrustedCurrentVisibilityAuthorization(t *testing.T) {
	now := time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)
	value := researchcontext.Context{
		SchemaVersion: researchcontext.SchemaVersion, ContextID: "ctx-protected", ContextVersion: 1,
		ResearchQuestion: "受控 Context", Scope: researchcontext.ScopeStock, SubjectRefs: []string{"security:600000"},
		SystemAsOf: now, SectionRefs: []researchcontext.SectionRef{{
			SectionID: "overview", SectionVersion: "1", SourceOwner: researchcontext.SourceOwnerResearch,
			SourceRefs: []string{}, EvidenceRefs: []string{}, QualityStatus: researchcontext.QualityValid,
			Required: true, Included: true, AssembledAt: now, SystemAsOf: now,
		}},
		QualitySummary: researchcontext.QualitySummary{Status: researchcontext.QualityValid, ReasonCodes: []string{}},
		IncludedSections: []string{"overview"}, OmittedSections: []researchcontext.OmittedSection{},
		BuilderVersion: "1.0.0", CreatedAt: now,
	}
	repo := &contextRepoFake{value: value}
	request := ContextReadRequest{Ref: ContextRef{ContextID: value.ContextID, ContextVersion: value.ContextVersion}, CallerRef: "researchflow:worker", PurposeCode: string(researchevidence.PurposeResearchAnalysis)}
	service, err := NewService(repo, &evidenceRepoFake{}, &authorizerFake{}, nil, nil)
	if err != nil { t.Fatal(err) }
	if _, err := service.GetContext(context.Background(), request); err == nil {
		t.Fatal("Context read without a current visibility authorizer must fail closed")
	}
	access := &contextAuthorizerFake{}
	service, err = NewService(repo, &evidenceRepoFake{}, &authorizerFake{}, nil, nil, access)
	if err != nil { t.Fatal(err) }
	got, err := service.GetContext(context.Background(), request)
	if err != nil { t.Fatalf("authorized Context read failed: %v", err) }
	if got.ContextID != value.ContextID || access.calls != 1 || access.caller != request.CallerRef ||
		access.purpose != researchevidence.PurposeResearchAnalysis || access.value.Scope != value.Scope ||
		!slices.Equal(access.value.SubjectRefs, value.SubjectRefs) {
		t.Fatalf("authorization did not receive exact caller/purpose/visibility: got=%+v authorizer=%+v", got, access)
	}
	access.err = NewAuthorizationError(AuthorizationDenied, errors.New("not visible"))
	if _, err := service.GetContext(context.Background(), request); err == nil {
		t.Fatal("explicit visibility denial must reject Context read")
	}
}

func TestCreateContextPreservesFixedDataNewsEvidenceAndOmissionFixture(t *testing.T) {
	now := time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)
	repo := &contextRepoFake{}
	service, err := NewService(repo, &evidenceRepoFake{}, &authorizerFake{}, func() time.Time { return now }, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	command := CreateContextCommand{
		ResearchQuestion: "浦发银行盈利质量是否改善",
		Scope:            string(researchcontext.ScopeStock),
		SubjectRefs:      []string{"security:600000"},
		SystemAsOf:       now,
		SectionRefs: []ContextSectionRef{
			{
				SectionID: "daily_bars", SectionVersion: "snapshot-v2",
				SourceOwner:   string(researchcontext.SourceOwnerData),
				SourceRefs:    []string{"data:snapshot:daily:600000:2026-11-11"},
				EvidenceRefs:  []string{"ev_data_daily_001"},
				QualityStatus: string(researchcontext.QualityDegraded),
				Required:      true, Included: true, AssembledAt: now, SystemAsOf: now,
			},
			{
				SectionID: "news_evidence", SectionVersion: "news-page-v1",
				SourceOwner:   string(researchcontext.SourceOwnerNews),
				SourceRefs:    []string{"news:snapshot:news_001:v3"},
				EvidenceRefs:  []string{"ev_news_001"},
				QualityStatus: string(researchcontext.QualityValid),
				Required:      true, Included: true, AssembledAt: now, SystemAsOf: now,
			},
			{
				SectionID: "quant_evaluation", SectionVersion: "evaluation-request-v1",
				SourceOwner:   string(researchcontext.SourceOwnerQuant),
				SourceRefs:    []string{"quant:request:evaluation:600000"},
				EvidenceRefs:  []string{},
				QualityStatus: string(researchcontext.QualityUnknown),
				Required:      false, Included: false, AssembledAt: now, SystemAsOf: now,
			},
		},
		QualitySummary: ContextQualitySummary{
			Status:               string(researchcontext.QualityDegraded),
			MissingRequiredCount: 0, MissingOptionalCount: 1,
			ConflictCount: 0, StaleCount: 0,
			ReasonCodes: []string{"data_quality_degraded", "optional_quant_unavailable"},
		},
		IncludedSections: []string{"daily_bars", "news_evidence"},
		OmittedSections: []ContextOmittedSection{{
			SectionID: "quant_evaluation", SourceOwner: string(researchcontext.SourceOwnerQuant),
			ReasonCode: "unavailable", Required: false,
			SourceRefs:  []string{"quant:request:evaluation:600000"},
			SafeMessage: "Quant evaluation is unavailable at the Run cutoff",
			Retryable:   true,
		}},
		BuilderVersion: "researchflow-context-fixture/1.0.0",
	}
	got, err := service.CreateContext(context.Background(), command)
	if err != nil {
		t.Fatalf("CreateContext() rejected valid fixed fixture: %v", err)
	}
	if len(repo.value.SectionRefs) != 3 || len(got.SectionRefs) != 3 ||
		len(got.SectionRefs[0].EvidenceRefs) != 1 || got.SectionRefs[0].EvidenceRefs[0] != "ev_data_daily_001" ||
		got.SectionRefs[1].EvidenceRefs[0] != "ev_news_001" ||
		got.SectionRefs[2].Included || got.QualitySummary.Status != string(researchcontext.QualityDegraded) {
		t.Fatalf("Context fixture references or quality state were not preserved: %+v", got)
	}
	if !got.SystemAsOf.Equal(now) || !got.SectionRefs[0].SystemAsOf.Equal(now) ||
		!got.SectionRefs[1].SystemAsOf.Equal(now) || !got.SectionRefs[2].SystemAsOf.Equal(now) {
		t.Fatal("Context and all source sections must preserve the exact Run system_as_of")
	}
}
func TestAddEvidenceGeneratesIdentityFingerprintAndRequiresCurrentAuthorization(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	repo := &evidenceRepoFake{}
	auth := &authorizerFake{}
	service, err := NewService(&contextRepoFake{}, repo, auth, func() time.Time { return now }, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	draft := researchevidence.Evidence{
		EvidenceType: researchevidence.TypeFact, SourceOwner: researchevidence.SourceData, SourceRef: "data:fact:1",
		DerivedFromRefs: []string{}, SubjectRefs: []string{"security:600000"}, SystemObservedAt: now, SystemAsOf: now,
		Stance: researchevidence.StanceNeutral, QualityStatus: researchevidence.QualityValid,
		ContentLevel: researchevidence.ContentMetadata, AuthorizationRef: "auth:1", Limitations: []string{},
	}
	got, err := service.AddEvidence(context.Background(), AddEvidenceCommand{CallerRef: "researchflow:worker-1", Draft: EvidenceDraft{EvidenceType: string(draft.EvidenceType), SourceOwner: string(draft.SourceOwner), SourceRef: draft.SourceRef, DerivedFromRefs: draft.DerivedFromRefs, SubjectRefs: draft.SubjectRefs, SystemObservedAt: draft.SystemObservedAt, SystemAsOf: draft.SystemAsOf, Stance: string(draft.Stance), QualityStatus: string(draft.QualityStatus), ContentLevel: string(draft.ContentLevel), AuthorizationRef: draft.AuthorizationRef, Limitations: draft.Limitations}})
	if err != nil {
		t.Fatalf("AddEvidence() error = %v, cause = %v", err, errors.Unwrap(err))
	}
	if got.EvidenceID != "ev_test-1" || got.ContentFingerprint == "" || !repo.appended || auth.calls != 1 {
		t.Fatalf("Evidence application flow incomplete: %+v", got)
	}
	if _, err := service.GetEvidence(context.Background(), EvidenceRef{EvidenceID: got.EvidenceID, CallerRef: "researchflow:worker-1"}); err != nil {
		t.Fatalf("authorized Evidence read failed: %v", err)
	}
	auth.err = errors.New("revoked")
	if _, err := service.GetEvidence(context.Background(), EvidenceRef{EvidenceID: got.EvidenceID, CallerRef: "researchflow:worker-1"}); err == nil {
		t.Fatal("revoked Evidence must not be returned")
	}
	repo.appended = false
	if _, err := service.AddEvidence(context.Background(), AddEvidenceCommand{CallerRef: "researchflow:worker-1", Draft: EvidenceDraft{EvidenceType: string(draft.EvidenceType), SourceOwner: string(draft.SourceOwner), SourceRef: draft.SourceRef, DerivedFromRefs: draft.DerivedFromRefs, SubjectRefs: draft.SubjectRefs, SystemObservedAt: draft.SystemObservedAt, SystemAsOf: draft.SystemAsOf, Stance: string(draft.Stance), QualityStatus: string(draft.QualityStatus), ContentLevel: string(draft.ContentLevel), AuthorizationRef: draft.AuthorizationRef, Limitations: draft.Limitations}}); err == nil {
		t.Fatal("revoked Evidence authorization must be rejected")
	}
	if repo.appended {
		t.Fatal("unauthorized Evidence must not be persisted")
	}
}
func TestNewServiceRequiresSourceOwnerAuthorizationAdapter(t *testing.T) {
	_, err := NewService(&contextRepoFake{}, &evidenceRepoFake{}, nil, nil, nil)
	if err == nil {
		t.Fatal("missing EvidenceAuthorizer must be rejected")
	}
}

type eventRepoFake struct {
	value             researchevent.ResearchEvent
	appended          bool
	lifecycleAppended bool
	correlationID     string
	reasonCode        *string
	appendErr         error
	getErr            error
}

func (f *eventRepoFake) Append(_ context.Context, value researchevent.ResearchEvent) error {
	f.value = value
	f.appended = true
	return f.appendErr
}
func (f *eventRepoFake) AppendWithLifecycle(_ context.Context, value researchevent.ResearchEvent, reasonCode *string, correlationID string) error {
	f.value = value
	f.appended = true
	f.lifecycleAppended = true
	f.reasonCode = reasonCode
	f.correlationID = correlationID
	return f.appendErr
}
func (f *eventRepoFake) GetVersion(_ context.Context, id string, version int) (researchevent.ResearchEvent, error) {
	if f.getErr != nil {
		return researchevent.ResearchEvent{}, f.getErr
	}
	if f.value.EventID != id || f.value.EventVersion != version {
		return researchevent.ResearchEvent{}, errors.New("missing")
	}
	return f.value, nil
}

func TestCreateAndGetEventUseApplicationDTOAndServerIdentity(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	repo := &eventRepoFake{}
	evidence := &evidenceRepoFake{values: map[string]researchevidence.Evidence{
		"ev_source_1": evidenceFixture(t, "ev_source_1", now),
	}}
	service, err := NewServiceWithEvents(&contextRepoFake{}, evidence, repo, &authorizerFake{}, func() time.Time { return now }, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	command := CreateEventCommand{
		EventType: "earnings_guidance", Title: "公司调整年度业绩指引", Status: string(researchevent.StatusDetected),
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{"ev_source_1"},
		SystemObservedAt: now, SystemAsOf: now, ImpactHypothesisRefs: []string{},
		InvalidationConditions: []string{"后续公告撤回该指引"}, Uncertainty: []string{}, TimeNormalizationRuleVersion: "1.0.0", DerivedFromRefs: []EventRef{}, SourceTimes: []EventSourceTime{{EvidenceRef: "ev_source_1", TimeType: string(researchevent.TimeSystemObservedAt), Value: now, AssertedBy: "news:observation"}},
	}
	createCtx, err := WithCorrelationID(context.Background(), "corr_create_parent")
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.CreateEvent(createCtx, command)
	if err != nil {
		t.Fatalf("CreateEvent() error = %v, cause = %v", err, errors.Unwrap(err))
	}
	if got.EventID != "evt_test-1" || got.EventVersion != 1 || !repo.appended || !repo.lifecycleAppended {
		t.Fatalf("Event was not created with server identity and lifecycle Outbox: %+v", got)
	}
	if repo.correlationID != "corr_create_parent" || repo.reasonCode != nil {
		t.Fatalf("initial lifecycle metadata mismatch: %+v", repo)
	}
	read, err := service.GetEvent(context.Background(), EventRef{EventID: got.EventID, EventVersion: 1})
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}
	if read.EventID != got.EventID || read.Status != command.Status {
		t.Fatalf("unexpected Event DTO: %+v", read)
	}
}

func TestUpdateEventUsesExpectedVersionAndDomainLifecycleRules(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	previous := researchevent.ResearchEvent{
		EventID: "evt_existing", EventVersion: 1, EventType: "earnings_guidance",
		Title: "公司调整年度业绩指引", Status: researchevent.StatusDetected,
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{"ev_source_1"},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{"公告撤回"},
		Uncertainty: []string{}, TimeNormalizationRuleVersion: "1.0.0", DerivedFromRefs: []researchevent.ResearchEventRef{}, SourceTimes: []researchevent.EventSourceTime{{
			EvidenceRef: "ev_source_1", TimeType: researchevent.TimeSystemObservedAt,
			Value: now, AssertedBy: "news:observation",
		}},
	}
	repo := &eventRepoFake{value: previous}
	evidence := &evidenceRepoFake{values: map[string]researchevidence.Evidence{
		"ev_source_1": evidenceFixture(t, "ev_source_1", now),
	}}
	service, err := NewServiceWithEvents(&contextRepoFake{}, evidence, repo, &authorizerFake{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	revision := CreateEventCommand{
		EventType: previous.EventType, Title: "业绩指引进入汇总阶段",
		Status:      string(researchevent.StatusConsolidating),
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{"ev_source_1"},
		SystemObservedAt: now, SystemAsOf: now.Add(time.Minute),
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{"公告撤回"},
		Uncertainty: []string{}, TimeNormalizationRuleVersion: "1.0.0", DerivedFromRefs: []EventRef{}, SourceTimes: []EventSourceTime{{
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
	if repo.appended {
		t.Fatal("invalid reason_code must not append an Event")
	}
	got, err := service.UpdateEvent(context.Background(), UpdateEventCommand{
		EventID: previous.EventID, ExpectedVersion: 1, Revision: revision,
	})
	if err != nil {
		t.Fatalf("UpdateEvent() error = %v, cause = %v", err, errors.Unwrap(err))
	}
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
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.UpdateEvent(context.Background(), UpdateEventCommand{EventID: "evt_missing", ExpectedVersion: 1})
	if err == nil || !errors.Is(err, researcherrors.ErrConflict) {
		t.Fatalf("missing expected version error = %v, want conflict fault", err)
	}
	if repo.appended {
		t.Fatal("missing expected version must not append a new Event")
	}
}

func TestCreateEventRejectsInvalidEventBeforePersistence(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	repo := &eventRepoFake{}
	service, err := NewServiceWithEvents(&contextRepoFake{}, &evidenceRepoFake{}, repo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateEvent(context.Background(), CreateEventCommand{EventType: "", Title: "", Status: "unknown", SystemObservedAt: now, SystemAsOf: now, SubjectRefs: []string{}, EvidenceRefs: []string{}, ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{}, Uncertainty: []string{}, TimeNormalizationRuleVersion: "1.0.0", DerivedFromRefs: []EventRef{}, SourceTimes: []EventSourceTime{}})
	if err == nil {
		t.Fatal("invalid Event must be rejected")
	}
	if repo.appended {
		t.Fatal("invalid Event must not be persisted")
	}
}

type claimRepoFake struct {
	value     researchclaim.Claim
	appended  bool
	appendErr error
}

func (f *claimRepoFake) Append(_ context.Context, value researchclaim.Claim) error {
	if f.appendErr != nil {
		return f.appendErr
	}
	f.value = value
	f.appended = true
	return nil
}
func (f *claimRepoFake) GetVersion(_ context.Context, id string, version int) (researchclaim.Claim, error) {
	if f.value.ClaimID != id || f.value.ClaimVersion != version {
		return researchclaim.Claim{}, errors.New("missing")
	}
	return f.value, nil
}

func TestCreateAndGetHypothesisUseApplicationDTO(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	repo := &claimRepoFake{}
	evidence := &evidenceRepoFake{values: map[string]researchevidence.Evidence{
		"ev_support_1": evidenceFixtureWithStance(t, "ev_support_1", now, researchevidence.StanceSupports),
	}}
	service, err := NewServiceWithClaims(&contextRepoFake{}, evidence, repo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	method := "比较公告前后盈利预测变化"
	observation := "公告后分析师预测上调"
	command := CreateClaimCommand{
		Statement:     "业绩指引上调将改善未来两个季度盈利预期",
		EpistemicType: string(researchclaim.EpistemicHypothesis),
		ScopeRefs:     []string{"security:600000"}, SystemAsOf: now,
		SupportingEvidenceRefs: []string{"ev_support_1"}, ContradictingEvidenceRefs: []string{},
		RequiredDataRefs: []string{"data:forecast_revision"}, ValidationMethod: &method,
		ExpectedObservation: &observation, InvalidationConditions: []string{"后续预测持续下调"},
		Status: string(researchclaim.StatusProposed), Limitations: []string{},
	}
	got, err := service.CreateClaim(context.Background(), command)
	if err != nil {
		t.Fatalf("CreateClaim() error = %v, cause = %v", err, errors.Unwrap(err))
	}
	if got.ClaimID != "clm_test-1" || got.ClaimVersion != 1 || !repo.appended {
		t.Fatalf("Claim was not created: %+v", got)
	}
	read, err := service.GetClaim(context.Background(), ClaimRef{ClaimID: got.ClaimID, ClaimVersion: 1})
	if err != nil {
		t.Fatalf("GetClaim failed: %v", err)
	}
	if read.Statement != command.Statement || read.EpistemicType != command.EpistemicType {
		t.Fatalf("unexpected Claim DTO: %+v", read)
	}
}

func TestClaimAndEventRejectMissingOrNonPITEvidenceReferences(t *testing.T) {
	now := time.Date(2026, 11, 12, 1, 0, 0, 0, time.UTC)
	method, observation := "compare revisions", "forecast increases"
	claimCommand := CreateClaimCommand{
		Statement: "盈利改善假设", EpistemicType: string(researchclaim.EpistemicHypothesis),
		ScopeRefs: []string{"security:600000"}, SystemAsOf: now,
		SupportingEvidenceRefs: []string{"ev_missing"}, ContradictingEvidenceRefs: []string{},
		RequiredDataRefs: []string{}, ValidationMethod: &method, ExpectedObservation: &observation,
		InvalidationConditions: []string{"预测持续下调"}, Status: string(researchclaim.StatusProposed), Limitations: []string{},
	}
	claimRepo := &claimRepoFake{}
	claimService, err := NewServiceWithClaims(&contextRepoFake{}, &evidenceRepoFake{}, claimRepo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := claimService.CreateClaim(context.Background(), claimCommand); err == nil {
		t.Fatal("Claim with missing Research Evidence must be rejected")
	}
	if claimRepo.appended {
		t.Fatal("Claim with missing Research Evidence must not be persisted")
	}

	future := evidenceFixture(t, "ev_future", now.Add(time.Minute))
	evidenceRepo := &evidenceRepoFake{values: map[string]researchevidence.Evidence{"ev_future": future}}
	claimService, err = NewServiceWithClaims(&contextRepoFake{}, evidenceRepo, claimRepo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	claimCommand.SupportingEvidenceRefs = []string{"ev_future"}
	if _, err := claimService.CreateClaim(context.Background(), claimCommand); err == nil {
		t.Fatal("Claim must reject Evidence learned after its system_as_of")
	}
	if claimRepo.appended {
		t.Fatal("PIT-invalid Claim must not be persisted")
	}

	eventRepo := &eventRepoFake{}
	eventService, err := NewServiceWithEvents(&contextRepoFake{}, &evidenceRepoFake{}, eventRepo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = eventService.CreateEvent(context.Background(), CreateEventCommand{
		EventType: "earnings_guidance", Title: "公告事件", Status: string(researchevent.StatusDetected),
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{"ev_missing"},
		SystemObservedAt: now, SystemAsOf: now, ImpactHypothesisRefs: []string{},
		InvalidationConditions: []string{}, Uncertainty: []string{},
		TimeNormalizationRuleVersion: "1.0.0", DerivedFromRefs: []EventRef{},
		SourceTimes: []EventSourceTime{{EvidenceRef: "ev_missing", TimeType: string(researchevent.TimeSystemObservedAt), Value: now, AssertedBy: "news"}},
	})
	if err == nil {
		t.Fatal("Event with missing Research Evidence must be rejected")
	}
	if eventRepo.appended {
		t.Fatal("Event with missing Research Evidence must not be persisted")
	}
}

func TestCreateHypothesisRequiresValidationContract(t *testing.T) {
	now := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)
	repo := &claimRepoFake{}
	service, err := NewServiceWithClaims(&contextRepoFake{}, &evidenceRepoFake{}, repo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.CreateClaim(context.Background(), CreateClaimCommand{
		Statement: "待验证假设", EpistemicType: string(researchclaim.EpistemicHypothesis),
		ScopeRefs: []string{}, SystemAsOf: now, SupportingEvidenceRefs: []string{},
		ContradictingEvidenceRefs: []string{}, RequiredDataRefs: []string{},
		InvalidationConditions: []string{"条件失效"}, Status: string(researchclaim.StatusProposed), Limitations: []string{},
	})
	if err == nil {
		t.Fatal("Hypothesis without validation method/observation must be rejected")
	}
	if repo.appended {
		t.Fatal("invalid Hypothesis must not be persisted")
	}
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
	evidence := &evidenceRepoFake{values: map[string]researchevidence.Evidence{
		"ev_support_1": evidenceFixtureWithStance(t, "ev_support_1", now, researchevidence.StanceSupports),
	}}
	service, err := NewServiceWithClaims(&contextRepoFake{}, evidence, repo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	revision := CreateClaimCommand{
		Statement: "盈利改善判断进入验证阶段", EpistemicType: string(researchclaim.EpistemicHypothesis),
		ScopeRefs: []string{"security:600000"}, SystemAsOf: now,
		SupportingEvidenceRefs: []string{"ev_support_1"}, ContradictingEvidenceRefs: []string{},
		RequiredDataRefs: []string{"data:forecast"}, ValidationMethod: stringPointer("比较预测修订"),
		ExpectedObservation: stringPointer("预测上调"), InvalidationConditions: []string{"预测持续下调"},
		Status: string(researchclaim.StatusTesting), Limitations: []string{},
	}
	got, err := service.UpdateClaim(context.Background(), UpdateClaimCommand{ClaimID: "clm_existing", ExpectedVersion: 1, Revision: revision})
	if err != nil {
		t.Fatalf("UpdateClaim() error = %v, cause = %v", err, errors.Unwrap(err))
	}
	if got.ClaimID != "clm_existing" || got.ClaimVersion != 2 || got.Status != string(researchclaim.StatusTesting) {
		t.Fatalf("unexpected Claim revision: %+v", got)
	}
	if _, err := service.UpdateClaim(context.Background(), UpdateClaimCommand{ClaimID: "clm_existing", ExpectedVersion: 1, Revision: revision}); err == nil {
		t.Fatal("stale expected version must not be accepted")
	}
	terminal := revision
	terminal.Status = string(researchclaim.StatusSupported)
	terminalResult, err := service.UpdateClaim(context.Background(), UpdateClaimCommand{ClaimID: "clm_existing", ExpectedVersion: 2, Revision: terminal})
	if err != nil {
		t.Fatalf("testing Claim must be allowed to reach supported: %v", err)
	}
	if terminalResult.ClaimVersion != 3 || terminalResult.Status != string(researchclaim.StatusSupported) {
		t.Fatalf("unexpected terminal Claim: %+v", terminalResult)
	}
	postTerminal := terminal
	postTerminal.Status = string(researchclaim.StatusContradicted)
	if _, err := service.UpdateClaim(context.Background(), UpdateClaimCommand{ClaimID: "clm_existing", ExpectedVersion: 3, Revision: postTerminal}); err == nil {
		t.Fatal("terminal Claim must not transition to another status")
	}
	if repo.value.ClaimVersion != 3 || repo.value.Status != researchclaim.StatusSupported {
		t.Fatal("invalid post-terminal transition must not be persisted")
	}
}

func stringPointer(value string) *string { return &value }

type mismatchedContextRepo struct{ contextRepoFake }

func (f *mismatchedContextRepo) GetVersion(context.Context, string, int64) (researchcontext.Context, error) {
	return f.value, nil
}

type mismatchedEventRepo struct{ eventRepoFake }

func (f *mismatchedEventRepo) GetVersion(context.Context, string, int) (researchevent.ResearchEvent, error) {
	return f.value, nil
}

type mismatchedClaimRepo struct{ claimRepoFake }

func (f *mismatchedClaimRepo) GetVersion(context.Context, string, int) (researchclaim.Claim, error) {
	return f.value, nil
}

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
	service, err := NewService(contextRepo, &evidenceRepoFake{}, &authorizerFake{}, nil, nil, &contextAuthorizerFake{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetContext(context.Background(), ContextReadRequest{Ref: ContextRef{ContextID: "ctx_requested", ContextVersion: 1}, CallerRef: "researchflow:worker", PurposeCode: string(researchevidence.PurposeResearchAnalysis)}); err == nil {
		t.Fatal("Context repository identity mismatch must be rejected")
	}

	eventRepo := &mismatchedEventRepo{eventRepoFake: eventRepoFake{value: researchevent.ResearchEvent{
		EventID: "evt_other", EventVersion: 1, EventType: "earnings_guidance", Title: "快照",
		Status: researchevent.StatusDetected, SubjectRefs: []string{"security:600000"},
		EvidenceRefs: []string{"ev_source_1"}, SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{}, Uncertainty: []string{}, TimeNormalizationRuleVersion: "1.0.0", DerivedFromRefs: []researchevent.ResearchEventRef{}, SourceTimes: []researchevent.EventSourceTime{{EvidenceRef: "ev_source_1", TimeType: researchevent.TimeSystemObservedAt, Value: now, AssertedBy: "news"}},
	}}}
	service, err = NewServiceWithEvents(&contextRepoFake{}, &evidenceRepoFake{}, eventRepo, &authorizerFake{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetClaim(context.Background(), ClaimRef{ClaimID: "clm_requested", ClaimVersion: 1}); err == nil {
		t.Fatal("Claim repository identity mismatch must be rejected")
	}
}

type mutatingEvidenceAuthorizer struct{}

func (mutatingEvidenceAuthorizer) AuthorizeEvidence(_ context.Context, _ string, _ researchevidence.PurposeCode, value researchevidence.Evidence) error {
	if value.SourceVersion != nil {
		*value.SourceVersion = "mutated"
	}
	if value.SourceLocator != nil {
		*value.SourceLocator = "mutated"
	}
	if value.ClaimRef != nil {
		*value.ClaimRef = "mutated"
	}
	if value.Confidence != nil {
		*value.Confidence = 0.99
	}
	if value.ConfidenceMethod != nil {
		*value.ConfidenceMethod = "mutated"
	}
	if value.ConfidenceMethodVersion != nil {
		*value.ConfidenceMethodVersion = "mutated"
	}
	if value.PublishedAt != nil {
		*value.PublishedAt = value.PublishedAt.Add(time.Hour)
	}
	if value.AvailableAt != nil {
		*value.AvailableAt = value.AvailableAt.Add(time.Hour)
	}
	if value.ValidTimeStart != nil {
		*value.ValidTimeStart = value.ValidTimeStart.Add(time.Hour)
	}
	if value.ValidTimeEnd != nil {
		*value.ValidTimeEnd = value.ValidTimeEnd.Add(time.Hour)
	}
	if len(value.SubjectRefs) > 0 {
		value.SubjectRefs[0] = "mutated"
	}
	if len(value.DerivedFromRefs) > 0 {
		value.DerivedFromRefs[0] = "mutated"
	}
	if len(value.Limitations) > 0 {
		value.Limitations[0] = "mutated"
	}
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
	repo := &evidenceRepoFake{values: map[string]researchevidence.Evidence{
		"ev_parent": evidenceFixture(t, "ev_parent", now),
	}}
	service, err := NewService(&contextRepoFake{}, repo, mutatingEvidenceAuthorizer{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.AddEvidence(context.Background(), AddEvidenceCommand{CallerRef: "researchflow:worker", Draft: draft})
	if err != nil {
		t.Fatal(err)
	}
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

type mismatchedEvidenceRepo struct{ evidenceRepoFake }

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
	if err != nil {
		t.Fatal(err)
	}
	value.ContentFingerprint = fingerprint
	repo := &mismatchedEvidenceRepo{evidenceRepoFake: evidenceRepoFake{value: value}}
	service, err := NewService(&contextRepoFake{}, repo, &authorizerFake{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.GetEvidence(context.Background(), EvidenceRef{
		EvidenceID: "ev_requested", CallerRef: "researchflow:worker",
	}); err == nil {
		t.Fatal("Evidence repository identity mismatch must be rejected")
	}
}

func TestCloneStringsAlwaysReturnsNonNilIndependentSlice(t *testing.T) {
	for _, input := range [][]string{nil, {}} {
		got := cloneStrings(input)
		if got == nil {
			t.Fatal("cloneStrings must normalize nil and empty input to a non-nil empty slice")
		}
	}
	input := []string{"evidence:1"}
	got := cloneStrings(input)
	got[0] = "changed"
	if input[0] != "evidence:1" {
		t.Fatal("cloneStrings must return an independent copy")
	}
}

func TestClaimRejectsEvidenceWithMismatchedStance(t *testing.T) {
	now := time.Date(2026, 11, 12, 2, 0, 0, 0, time.UTC)
	method, observation := "compare revisions", "forecast increases"
	base := CreateClaimCommand{
		Statement: "盈利改善假设", EpistemicType: string(researchclaim.EpistemicHypothesis),
		ScopeRefs: []string{"security:600000"}, SystemAsOf: now,
		SupportingEvidenceRefs: []string{"ev_support"}, ContradictingEvidenceRefs: []string{"ev_contradict"},
		RequiredDataRefs: []string{}, ValidationMethod: &method, ExpectedObservation: &observation,
		InvalidationConditions: []string{"预测持续下调"}, Status: string(researchclaim.StatusProposed), Limitations: []string{},
	}
	for _, tc := range []struct {
		name          string
		supporting    researchevidence.Stance
		contradicting researchevidence.Stance
	}{
		{name: "support reference cannot point to contradiction evidence", supporting: researchevidence.StanceContradicts, contradicting: researchevidence.StanceContradicts},
		{name: "contradiction reference cannot point to support evidence", supporting: researchevidence.StanceSupports, contradicting: researchevidence.StanceSupports},
		{name: "neutral evidence cannot be classified as support or contradiction", supporting: researchevidence.StanceNeutral, contradicting: researchevidence.StanceContradicts},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &claimRepoFake{}
			evidence := &evidenceRepoFake{values: map[string]researchevidence.Evidence{
				"ev_support": evidenceFixtureWithStance(t, "ev_support", now, tc.supporting),
				"ev_contradict": evidenceFixtureWithStance(t, "ev_contradict", now, tc.contradicting),
			}}
			service, err := NewServiceWithClaims(&contextRepoFake{}, evidence, repo, &authorizerFake{}, nil, &fixedIDs{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := service.CreateClaim(context.Background(), base); err == nil {
				t.Fatal("Claim must reject Evidence whose stance conflicts with its reference list")
			}
			if repo.appended {
				t.Fatal("Claim with mismatched Evidence stance must not be persisted")
			}
		})
	}
}

func TestUpdateClaimRejectsEvidenceWithMismatchedStance(t *testing.T) {
	now := time.Date(2026, 11, 12, 3, 0, 0, 0, time.UTC)
	previous := researchclaim.Claim{
		ClaimID: "clm_existing", ClaimVersion: 1, Statement: "待验证的盈利改善判断",
		EpistemicType: researchclaim.EpistemicHypothesis, ScopeRefs: []string{"security:600000"},
		SystemAsOf: now.Add(-time.Minute), SupportingEvidenceRefs: []string{},
		ContradictingEvidenceRefs: []string{}, RequiredDataRefs: []string{},
		ValidationMethod: stringPointer("比较预测修订"), ExpectedObservation: stringPointer("预测上调"),
		InvalidationConditions: []string{"预测持续下调"}, Status: researchclaim.StatusProposed, Limitations: []string{},
	}
	repo := &claimRepoFake{value: previous}
	evidence := &evidenceRepoFake{values: map[string]researchevidence.Evidence{
		"ev_contradict": evidenceFixtureWithStance(t, "ev_contradict", now, researchevidence.StanceContradicts),
	}}
	service, err := NewServiceWithClaims(&contextRepoFake{}, evidence, repo, &authorizerFake{}, nil, &fixedIDs{})
	if err != nil {
		t.Fatal(err)
	}
	revision := CreateClaimCommand{
		Statement: "盈利改善判断进入验证阶段", EpistemicType: string(researchclaim.EpistemicHypothesis),
		ScopeRefs: []string{"security:600000"}, SystemAsOf: now,
		SupportingEvidenceRefs: []string{"ev_contradict"}, ContradictingEvidenceRefs: []string{},
		RequiredDataRefs: []string{}, ValidationMethod: stringPointer("比较预测修订"),
		ExpectedObservation: stringPointer("预测上调"), InvalidationConditions: []string{"预测持续下调"},
		Status: string(researchclaim.StatusTesting), Limitations: []string{},
	}
	if _, err := service.UpdateClaim(context.Background(), UpdateClaimCommand{
		ClaimID: previous.ClaimID, ExpectedVersion: previous.ClaimVersion, Revision: revision,
	}); err == nil {
		t.Fatal("UpdateClaim must reject Evidence whose stance conflicts with its reference list")
	}
	if repo.appended || repo.value.ClaimVersion != previous.ClaimVersion {
		t.Fatal("Claim with mismatched Evidence stance must not be appended")
	}
}
