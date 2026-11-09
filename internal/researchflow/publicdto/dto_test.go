package publicdto

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

func TestRunDTOUsesPublicNamesAndExplicitNulls(t *testing.T) {
	now:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	got:=RunFromDomain(domain.Run{Ref:"run_1",SchemaVersion:"1.0.0",Status:domain.RunCreated,SystemAsOf:now,DeadlineAt:now.Add(time.Hour),TemplateRef:"daily",TemplateVersion:"1",CreatedAt:now})
	raw,err:=json.Marshal(got); if err!=nil{t.Fatal(err)}
	var fields map[string]any; if err=json.Unmarshal(raw,&fields);err!=nil{t.Fatal(err)}
	for _,key:=range []string{"run_ref","result_summary_ref","originating_request_id","termination_reason","started_at","finished_at"} {
		if _,ok:=fields[key];!ok{t.Errorf("public field %q missing",key)}
	}
	if fields["result_summary_ref"]!=nil || fields["started_at"]!=nil {t.Fatalf("nullable fields must be explicit nulls: %s",raw)}
	if _,ok:=fields["ref"];ok{t.Fatal("domain field name leaked into public DTO")}
}

func TestStageAndAttemptDTOUseStablePublicFieldNames(t *testing.T) {
	stage:=StageFromDomain(domain.Stage{Ref:"stage_1",RunRef:"run_1",Code:"quote",Status:domain.StageReady}, "1.0.0")
	attempt:=AttemptFromDomain(domain.Attempt{Ref:"attempt_1",StageRef:"stage_1",Number:1,Status:domain.AttemptRunning}, "1.0.0")
	sraw,_:=json.Marshal(stage); araw,_:=json.Marshal(attempt)
	var s,a map[string]any; _=json.Unmarshal(sraw,&s); _=json.Unmarshal(araw,&a)
	if s["stage_ref"]!="stage_1" || s["dependency_stage_refs"]==nil || s["attempt_no"]!=nil || s["diagnostic_refs"]==nil {t.Fatalf("unexpected Stage DTO: %s",sraw)}
	if a["attempt_ref"]!="attempt_1" || a["attempt_no"]!=float64(1) || a["lease_owner"]!=nil {t.Fatalf("unexpected Attempt DTO: %s",araw)}
}


func TestPublicDTOSchemaGoldens(t *testing.T) {
	cases := []struct{ schema, golden string }{
		{"run-1.0.0.schema.json","run-created-1.0.0.golden.json"},
		{"run-1.0.0.schema.json","run-partial-1.0.0.golden.json"},
		{"run-1.0.0.schema.json","run-failed-1.0.0.golden.json"},
		{"run-1.0.0.schema.json","run-cancelled-1.0.0.golden.json"},
		{"result-summary-1.0.0.schema.json","result-summary-partial-1.0.0.golden.json"},
		{"stage-1.0.0.schema.json","stage-ready-1.0.0.golden.json"},
		{"stage-1.0.0.schema.json","stage-skipped-1.0.0.golden.json"},
		{"stage-1.0.0.schema.json","stage-degraded-1.0.0.golden.json"},
		{"stage-1.0.0.schema.json","stage-failed-1.0.0.golden.json"},
		{"attempt-1.0.0.schema.json","attempt-running-1.0.0.golden.json"},
		{"attempt-1.0.0.schema.json","attempt-failed-1.0.0.golden.json"},
	}
	for _, tc := range cases {
		t.Run(tc.golden,func(t *testing.T){
			schemaBytes,err:=os.ReadFile(filepath.Join("../../../doc/researchflow/schema",tc.schema)); if err!=nil{t.Fatal(err)}
			goldenBytes,err:=os.ReadFile(filepath.Join("../../../doc/researchflow/golden",tc.golden)); if err!=nil{t.Fatal(err)}
			compiler:=jsonschema.NewCompiler()
			compiler.Draft=jsonschema.Draft2020
			compiler.AssertFormat=true
			const schemaURL="https://researchflow.invalid/schema/golden-test"
			if err:=compiler.AddResource(schemaURL,bytes.NewReader(schemaBytes));err!=nil{t.Fatal(err)}
			schema,err:=compiler.Compile(schemaURL);if err!=nil{t.Fatal(err)}
			decoder:=json.NewDecoder(bytes.NewReader(goldenBytes))
			decoder.UseNumber()
			var instance any
			if err:=decoder.Decode(&instance);err!=nil{t.Fatal(err)}
			if err:=schema.Validate(instance);err!=nil{t.Fatalf("golden rejected by public schema: %v",err)}
			var object map[string]any
			if err:=json.Unmarshal(goldenBytes,&object);err!=nil{t.Fatal(err)}
			object["unexpected_public_field"]=true
			if err:=schema.Validate(object);err==nil{t.Fatal("schema accepted an undeclared public field")}
		})
	}
}

func TestRunDTOClonesNestedResultSummaryAndNormalizesCollections(t *testing.T) {
	summary:=&domain.ResultSummary{StageOutcomes:map[string]domain.StageStatus{"quote":domain.StageSucceeded},RiskGateDecisions:map[string]string{"risk":"pass"}}
	got:=ResultSummaryFromDomain(summary)
	if got==nil || got.StageOutcomes["quote"]!="succeeded" || got.RiskGateDecisions["risk"]!="pass" {
		t.Fatalf("result summary mapping lost values: %#v",got)
	}
	if got.ExcludedStages==nil || got.MissingStageCodes==nil || got.ReasonCodes==nil {
		t.Fatalf("public collections must be arrays, not null: %#v",got)
	}
	got.StageOutcomes["quote"]="failed"
	if summary.StageOutcomes["quote"]!=domain.StageSucceeded {t.Fatal("DTO mapping must not share mutable map with domain")}
}


func TestDomainProjectionsValidateAgainstTheirPublicSchemas(t *testing.T) {
	now:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	run:=RunFromDomain(domain.Run{
		Ref:"run_schema",SchemaVersion:"1.0.0",Status:domain.RunPartial,Version:2,
		SystemAsOf:now,DeadlineAt:now.Add(time.Hour),TemplateRef:"daily",TemplateVersion:"1.0.0",
		TemplateFingerprint:"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		InputSnapshotRefs:[]string{"snapshot_1"},CorrelationID:"corr_1",CreatedAt:now,
		StartedAt:&now,FinishedAt:timePointer(now.Add(time.Minute)),
		ResultSummaryRef:stringPointer("summary_1"),ResultSummary:&domain.ResultSummary{
			StageOutcomes:map[string]domain.StageStatus{"quote":domain.StageDegraded},
			ExcludedStages:[]domain.ExcludedStage{},MissingStageCodes:[]string{"risk"},
			DegradedStageCodes:[]string{"quote"},BlockedStageCodes:[]string{},ReasonCodes:[]string{"source_partial"},
			Coverage:0.5,QualityStatus:"incomplete",RiskGateDecisions:map[string]string{},
			ActionabilityStatus:"blocked",
		},
	})
	stage:=StageFromDomain(domain.Stage{
		Ref:"stage_schema",RunRef:"run_schema",Code:"quote",Status:domain.StageDegraded,Version:2,
		Required:false,Dependencies:[]string{},RetryPolicyRef:"retry_v1",AttemptNo:1,
		QualitySummaryRef:stringPointer("quality_1"),DiagnosticRefs:[]string{"diagnostic_1"},
		ReasonCodes:[]string{"source_partial"},CreatedAt:now,StartedAt:&now,FinishedAt:timePointer(now.Add(time.Second)),
	}, "1.0.0")
	attempt:=AttemptFromDomain(domain.Attempt{
		Ref:"attempt_schema",StageRef:"stage_schema",Number:1,Status:domain.AttemptFailed,
		LeaseOwner:"worker_1",FencingToken:1,StartedAt:now,FinishedAt:timePointer(now.Add(time.Second)),
		FaultRef:stringPointer("fault_1"),
	}, "1.0.0")

	assertDTO:=func(name,schemaPath string,value any){
		t.Helper()
		raw,err:=json.Marshal(value);if err!=nil{t.Fatalf("%s marshal: %v",name,err)}
		schemaBytes,err:=os.ReadFile(filepath.Join("../../../doc/researchflow/schema",schemaPath));if err!=nil{t.Fatal(err)}
		compiler:=jsonschema.NewCompiler();compiler.Draft=jsonschema.Draft2020;compiler.AssertFormat=true
		const schemaURL="https://researchflow.invalid/schema/runtime-test"
		if err:=compiler.AddResource(schemaURL,bytes.NewReader(schemaBytes));err!=nil{t.Fatal(err)}
		compiled,err:=compiler.Compile(schemaURL);if err!=nil{t.Fatal(err)}
		decoder:=json.NewDecoder(bytes.NewReader(raw));decoder.UseNumber()
		var instance any;if err:=decoder.Decode(&instance);err!=nil{t.Fatal(err)}
		if err:=compiled.Validate(instance);err!=nil{t.Fatalf("%s DTO violates public schema: %v; json=%s",name,err,raw)}
	}
	assertDTO("Run","run-1.0.0.schema.json",run)
	assertDTO("ResultSummary","result-summary-1.0.0.schema.json",ResultSummaryFromDomain(domainRunSummaryForTest()))
	assertDTO("Stage","stage-1.0.0.schema.json",stage)
	assertDTO("Attempt","attempt-1.0.0.schema.json",attempt)
}

func TestAttemptDTOClonesNullableTimePointers(t *testing.T) {
	finished:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	got:=AttemptFromDomain(domain.Attempt{FinishedAt:&finished},"1.0.0")
	*got.FinishedAt=got.FinishedAt.Add(time.Hour)
	if !finished.Equal(time.Date(2026,11,6,1,0,0,0,time.UTC)){t.Fatal("DTO must not share nullable time pointer with domain")}
}

func timePointer(v time.Time) *time.Time { return &v }

func domainRunSummaryForTest() *domain.ResultSummary {
	return &domain.ResultSummary{
		StageOutcomes:map[string]domain.StageStatus{"quote":domain.StageDegraded},
		ExcludedStages:[]domain.ExcludedStage{{StageCode:"optional_news",RuleVersion:"v2",ReasonCode:"market_closed"}},
		MissingStageCodes:[]string{"risk"},DegradedStageCodes:[]string{"quote"},
		BlockedStageCodes:[]string{},ReasonCodes:[]string{"source_partial"},
		Coverage:0.5,QualityStatus:"incomplete",RiskGateDecisions:map[string]string{"risk":"unknown"},
		ActionabilityStatus:"blocked",
	}
}

func TestTerminalRunSchemaRequiresSummaryReference(t *testing.T) {
	raw,err:=json.Marshal(RunFromDomain(domain.Run{SchemaVersion:"1.0.0",Ref:"run_terminal",Status:domain.RunFailed,Version:2}))
	if err!=nil{t.Fatal(err)}
	schemaBytes,err:=os.ReadFile(filepath.Join("../../../doc/researchflow/schema","run-1.0.0.schema.json"));if err!=nil{t.Fatal(err)}
	compiler:=jsonschema.NewCompiler();compiler.Draft=jsonschema.Draft2020
	const schemaURL="https://researchflow.invalid/schema/terminal-reference-test"
	if err:=compiler.AddResource(schemaURL,bytes.NewReader(schemaBytes));err!=nil{t.Fatal(err)}
	compiled,err:=compiler.Compile(schemaURL);if err!=nil{t.Fatal(err)}
	decoder:=json.NewDecoder(bytes.NewReader(raw));decoder.UseNumber()
	var instance any;if err:=decoder.Decode(&instance);err!=nil{t.Fatal(err)}
	if err:=compiled.Validate(instance);err==nil{t.Fatal("terminal Run without result_summary_ref passed public schema")}
}
