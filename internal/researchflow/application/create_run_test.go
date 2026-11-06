package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type identityStub struct { caller, correlation string; requestID *string }
func (s identityStub) AuthenticatedCallerRef(context.Context)(string,error){return s.caller,nil}
func (s identityStub) OriginatingRequestID(context.Context)*string{return s.requestID}
func (s identityStub) CorrelationID(context.Context)string{return s.correlation}

type clockStub struct { now time.Time }
func (s clockStub) NowUTC(context.Context)(time.Time,error){return s.now,nil}

type idsStub struct { next int }
func (s *idsStub) NewRef(_ context.Context, kind string)(string,error){s.next++;return kind+"_ref_"+string(rune('0'+s.next)),nil}

type templatesStub struct { value template.WorkflowTemplate; calls int }
func (s *templatesStub) Get(_ context.Context, ref, version string)(template.WorkflowTemplate,error){
	s.calls++
	if ref!=s.value.Ref||version!=s.value.Version{return template.WorkflowTemplate{},errors.New("not found")}
	return s.value,nil
}

type schemasStub struct{}
func (schemasStub) Validate(_ context.Context, ref string,_ json.RawMessage)(template.ParameterSchemaSnapshot,error){
	return template.ParameterSchemaSnapshot{Ref:ref,Version:"1",Fingerprint:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},nil
}
type applicabilityStub struct{}
func (applicabilityStub) Evaluate(_ context.Context,_ template.ApplicabilityRule,_ json.RawMessage)(bool,string,error){return true,"",nil}

type repositoryStub struct {
	existing *CreateRunResult
	existingFingerprint string
	created *CreateRunRecord
}
func (s *repositoryStub) FindIdempotentRun(_ context.Context,_ IdempotencyScope)(CreateRunResult,string,bool,error){
	if s.existing==nil{return CreateRunResult{},"",false,nil}
	return *s.existing,s.existingFingerprint,true,nil
}
func (s *repositoryStub) CreateRunAtomic(_ context.Context,record CreateRunRecord)(CreateRunResult,error){
	s.created=&record
	return CreateRunResult{Run:record.Run,Stages:record.Stages},nil
}

func validTemplate() template.WorkflowTemplate {
	retry:=template.RetryPolicy{Ref:"retry-v1",MaxAttempts:2,RetryableReasonCodes:[]string{"provider_unavailable"},Backoff:template.BackoffPolicy{Algorithm:"exponential_full_jitter_v1",BaseDelayMS:10,MaxDelayMS:100},AttemptTimeoutMS:1000}
	propagation:=template.PropagationPolicy{OnDegraded:domain.PropagateAccept,OnSkipped:domain.PropagatePartial,OnFailed:domain.PropagateFail,OnTimedOut:domain.PropagateFail,OnBlocked:domain.PropagateFail}
	return template.WorkflowTemplate{Ref:"daily",Version:"1.0.0",ParameterSchemaRef:"daily-params-v1",OutputRegistryVersion:"outputs-v1",ResourcePolicyRef:"resource-v1",Stages:[]template.StageDefinition{
		{Code:"quote",Required:true,Applicability:template.ApplicabilityRule{RuleRef:"always",RuleVersion:"1"},Retry:retry,Propagation:propagation},
		{Code:"research",Required:false,Dependencies:[]string{"quote"},Applicability:template.ApplicabilityRule{RuleRef:"always",RuleVersion:"1"},Retry:retry,Propagation:propagation},
	}}
}

func newService(t *testing.T, repo *repositoryStub, ids *idsStub, templateReader *templatesStub, now time.Time)(*CreateRunService,error){
	t.Helper()
	return NewCreateRunService(identityStub{caller:"caller_1"},clockStub{now},ids,templateReader,schemasStub{},applicabilityStub{},nil,repo)
}
func command(now time.Time)domain.CreateRunCommand{
	return domain.CreateRunCommand{SchemaVersion:"1.0.0",IdempotencyKey:"idem_1",TemplateRef:"daily",TemplateVersion:"1.0.0",SystemAsOf:now.Add(-time.Minute),DeadlineAt:now.Add(time.Hour),InputSnapshotRefs:[]string{"snapshot_1"},Parameters:json.RawMessage("{}")}
}

func TestCreateRunResolvesAndPersistsRunStagesAndSnapshot(t *testing.T){
	now:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	repo:=&repositoryStub{};ids:=&idsStub{};reader:=&templatesStub{value:validTemplate()}
	service,err:=newService(t,repo,ids,reader,now);if err!=nil{t.Fatal(err)}
	got,err:=service.CreateRun(context.Background(),command(now));if err!=nil{t.Fatal(err)}
	if got.Run.Status!=domain.RunCreated||got.Run.TemplateFingerprint==""||len(got.Stages)!=2{t.Fatalf("unexpected create result: %#v",got)}
	if repo.created==nil||repo.created.ResolvedTemplate.ParameterSchema.Fingerprint==""||string(repo.created.Parameters)!="{}"{t.Fatal("resolved template and parameters must be persisted with run")}
	if got.Stages[1].Dependencies[0]!=got.Stages[0].Ref{t.Fatal("stage dependency must use generated stage ref")}
	if got.Stages[0].Status!=domain.StagePending||got.Stages[0].AttemptNo!=0{t.Fatal("new stages must be pending without an Attempt")}
}

func TestCreateRunReplaysBeforeTemplateLookup(t *testing.T){
	now:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	cmd:=command(now)
	fingerprint,err:=domain.RequestFingerprint("caller_1",cmd);if err!=nil{t.Fatal(err)}
	existing:=CreateRunResult{Run:domain.Run{Ref:"run_existing",Status:domain.RunCreated}}
	repo:=&repositoryStub{existing:&existing,existingFingerprint:fingerprint}
	reader:=&templatesStub{value:validTemplate()}
	service,err:=newService(t,repo,&idsStub{},reader,now);if err!=nil{t.Fatal(err)}
	got,err:=service.CreateRun(context.Background(),cmd);if err!=nil{t.Fatal(err)}
	if !got.Replayed||got.Run.Ref!="run_existing"||reader.calls!=0{t.Fatalf("expected idempotent replay before resolving template: %#v, calls=%d",got,reader.calls)}
}

func TestCreateRunRejectsIdempotencyConflictBeforeTemplateLookup(t *testing.T){
	now:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	existing:=CreateRunResult{Run:domain.Run{Ref:"run_existing"}}
	repo:=&repositoryStub{existing:&existing,existingFingerprint:"different"}
	reader:=&templatesStub{value:validTemplate()}
	service,err:=newService(t,repo,&idsStub{},reader,now);if err!=nil{t.Fatal(err)}
	_,err=service.CreateRun(context.Background(),command(now))
	if !errors.Is(err,domain.ErrIdempotencyConflict)||reader.calls!=0{t.Fatalf("expected conflict before template lookup, got %v",err)}
}

func TestCreateRunFailsClosedForTemplateVersionMismatch(t *testing.T){
	now:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	tmpl:=validTemplate();tmpl.Version="2.0.0"
	repo:=&repositoryStub{};reader:=&templatesStub{value:tmpl}
	service,err:=newService(t,repo,&idsStub{},reader,now);if err!=nil{t.Fatal(err)}
	_,err=service.CreateRun(context.Background(),command(now))
	if !errors.Is(err,ErrTemplateVersionMismatch)||repo.created!=nil{t.Fatalf("mismatched template must not persist: %v",err)}
}

func TestCreateRunOverridesUntrustedTracingFields(t *testing.T){
	now:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	requestID:="trusted-request"
	repo:=&repositoryStub{};reader:=&templatesStub{value:validTemplate()}
	service,err:=NewCreateRunService(identityStub{caller:"caller_1",requestID:&requestID,correlation:"trusted-correlation"},clockStub{now},&idsStub{},reader,schemasStub{},applicabilityStub{},nil,repo)
	if err!=nil{t.Fatal(err)}
	cmd:=command(now);cmd.CorrelationID="client-spoof";spoof:="client-spoof";cmd.OriginatingRequestID=&spoof
	got,err:=service.CreateRun(context.Background(),cmd);if err!=nil{t.Fatal(err)}
	if got.Run.CorrelationID!="trusted-correlation"||got.Run.OriginatingRequestID==nil||*got.Run.OriginatingRequestID!="trusted-request"{t.Fatalf("trusted request metadata not injected: %#v",got.Run)}
}
