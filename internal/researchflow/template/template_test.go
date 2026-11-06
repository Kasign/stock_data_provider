package template

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type testEvaluator map[string]bool
func(e testEvaluator)Evaluate(_ context.Context,r ApplicabilityRule,_ json.RawMessage)(bool,string,error){if e[r.RuleRef]{return true,"",nil};return false,"not_applicable",nil}
type testGates map[string]bool
func(g testGates)Contains(code string)bool{return g[code]}

type testParameterSchemas struct{}
func(testParameterSchemas) Validate(_ context.Context, ref string, _ json.RawMessage) (ParameterSchemaSnapshot, error) {
	return ParameterSchemaSnapshot{Ref:ref, Version:"1", Fingerprint:"schema-sha256"}, nil
}

func testPolicy()PropagationPolicy{return PropagationPolicy{OnDegraded:domain.PropagateAccept,OnSkipped:domain.PropagatePartial,OnFailed:domain.PropagateFail,OnTimedOut:domain.PropagateFail,OnBlocked:domain.PropagateFail}}
func testRetry()RetryPolicy{return RetryPolicy{Ref:"retry-v1",MaxAttempts:2,RetryableReasonCodes:[]string{"provider_unavailable"},Backoff:BackoffPolicy{Algorithm:"exponential_full_jitter_v1",BaseDelayMS:100,MaxDelayMS:1000},AttemptTimeoutMS:5000}}
func testTemplate()WorkflowTemplate{return WorkflowTemplate{Ref:"stock_research",Version:"1",ParameterSchemaRef:"stock-research-params-v1",OutputRegistryVersion:"outputs-v1",ResourcePolicyRef:"resource-v1",Stages:[]StageDefinition{
	{Code:"market",Required:true,Applicability:ApplicabilityRule{RuleRef:"always",RuleVersion:"1"},Retry:testRetry(),Propagation:testPolicy()},
	{Code:"summary",Required:true,Dependencies:[]string{"market"},Applicability:ApplicabilityRule{RuleRef:"always",RuleVersion:"1"},Retry:testRetry(),Propagation:testPolicy()},
}}}

func TestValidateRejectsCyclesAndWildcardRetryCodes(t *testing.T){
	tmpl:=testTemplate();tmpl.Stages[0].Dependencies=[]string{"summary"}
	if err:=Validate(tmpl,nil);err==nil||!strings.Contains(err.Error(),"cycle"){t.Fatalf("expected cycle error, got %v",err)}
	tmpl=testTemplate();tmpl.Stages[0].Retry.RetryableReasonCodes=[]string{"provider_*"}
	if err:=Validate(tmpl,nil);err==nil{t.Fatal("wildcard retry code accepted")}
}

func TestResolveOrdersDAGAndRejectsExcludedDependency(t *testing.T){
	tmpl:=testTemplate()
	resolved,err:=Resolve(context.Background(),tmpl,json.RawMessage(`{}`),testParameterSchemas{},testEvaluator{"always":true},nil)
	if err!=nil{t.Fatal(err)}
	if len(resolved.Stages)!=2||resolved.Stages[0].Definition.Code!="market"||resolved.Stages[1].Definition.Code!="summary"{t.Fatalf("unexpected order: %#v",resolved.Stages)}
	tmpl.Stages[0].Applicability.RuleRef="market-only"
	_,err=Resolve(context.Background(),tmpl,json.RawMessage(`{}`),testParameterSchemas{},testEvaluator{"always":true},nil)
	if err==nil{t.Fatal("applicable stage depending on excluded stage was accepted")}
}

func TestHardRiskGateMustBeRegisteredAndFailClosed(t *testing.T){
	tmpl:=testTemplate();tmpl.RequiresHardRiskGate=true;tmpl.Stages[0].GateType="hard_risk"
	if err:=Validate(tmpl,testGates{});err==nil{t.Fatal("unregistered gate accepted")}
	if err:=Validate(tmpl,testGates{"market":true});err==nil{t.Fatal("non-fail-closed gate accepted")}
	p:=PropagationPolicy{OnDegraded:domain.PropagateFail,OnSkipped:domain.PropagateFail,OnFailed:domain.PropagateFail,OnTimedOut:domain.PropagateFail,OnBlocked:domain.PropagateFail}
	tmpl.Stages[0].Propagation=p
	if err:=Validate(tmpl,testGates{"market":true});err!=nil{t.Fatal(err)}
}


func TestFingerprintCanonicalizesJSONAndSetLikeCollections(t *testing.T) {
	a := testTemplate()
	b := testTemplate()
	b.Stages[0], b.Stages[1] = b.Stages[1], b.Stages[0]
	b.Stages[1].Dependencies = nil
	b.Stages[0].Dependencies = []string{"market"}
	fa, err := Fingerprint(a)
	if err != nil { t.Fatal(err) }
	fb, err := Fingerprint(b)
	if err != nil { t.Fatal(err) }
	if fa != fb { t.Fatalf("equivalent template collection order changed fingerprint: %s != %s", fa, fb) }
}


func TestResolveRequiresImmutableParameterSchemaSnapshot(t *testing.T) {
	tmpl := testTemplate()
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err != nil { t.Fatal(err) }
	if resolved.ParameterSchema.Ref != tmpl.ParameterSchemaRef || resolved.ParameterSchema.Version != "1" || resolved.ParameterSchema.Fingerprint == "" {
		t.Fatalf("parameter schema snapshot not preserved: %#v", resolved.ParameterSchema)
	}
	_, err = Resolve(context.Background(), tmpl, json.RawMessage("{}"), nil, testEvaluator{"always": true}, nil)
	if err == nil { t.Fatal("missing parameter schema validator accepted") }
}

type mismatchedParameterSchemas struct{}
func(mismatchedParameterSchemas) Validate(_ context.Context, _ string, _ json.RawMessage) (ParameterSchemaSnapshot, error) {
	return ParameterSchemaSnapshot{Ref:"other-schema", Version:"1", Fingerprint:"sha256"}, nil
}

func TestResolveRejectsMismatchedParameterSchemaRef(t *testing.T) {
	_, err := Resolve(context.Background(), testTemplate(), json.RawMessage("{}"), mismatchedParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err == nil { t.Fatal("mismatched parameter schema ref accepted") }
}
