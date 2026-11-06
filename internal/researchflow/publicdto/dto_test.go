package publicdto

import (
	"encoding/json"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

func TestRunDTOUsesPublicNamesAndExplicitNulls(t *testing.T) {
	now:=time.Date(2026,11,6,1,0,0,0,time.UTC)
	got:=RunFromDomain(domain.Run{Ref:"run_1",SchemaVersion:"1.0.0",Status:domain.RunCreated,SystemAsOf:now,DeadlineAt:now.Add(time.Hour),TemplateRef:"daily",TemplateVersion:"1",CreatedAt:now})
	raw,err:=json.Marshal(got); if err!=nil{t.Fatal(err)}
	var fields map[string]any; if err=json.Unmarshal(raw,&fields);err!=nil{t.Fatal(err)}
	for _,key:=range []string{"run_ref","result_summary","originating_request_id","termination_reason","started_at","finished_at"} {
		if _,ok:=fields[key];!ok{t.Errorf("public field %q missing",key)}
	}
	if fields["result_summary"]!=nil || fields["started_at"]!=nil {t.Fatalf("nullable fields must be explicit nulls: %s",raw)}
	if _,ok:=fields["ref"];ok{t.Fatal("domain field name leaked into public DTO")}
}

func TestStageAndAttemptDTOUseStablePublicFieldNames(t *testing.T) {
	stage:=StageFromDomain(domain.Stage{Ref:"stage_1",RunRef:"run_1",Code:"quote",Status:domain.StageReady}, "1.0.0")
	attempt:=AttemptFromDomain(domain.Attempt{Ref:"attempt_1",StageRef:"stage_1",Status:domain.AttemptRunning}, "1.0.0")
	sraw,_:=json.Marshal(stage); araw,_:=json.Marshal(attempt)
	var s,a map[string]any; _=json.Unmarshal(sraw,&s); _=json.Unmarshal(araw,&a)
	if s["stage_ref"]!="stage_1" || s["dependency_stage_refs"]==nil {t.Fatalf("unexpected Stage DTO: %s",sraw)}
	if a["attempt_ref"]!="attempt_1" || a["attempt_no"]!=float64(0) || a["lease_owner"]!=nil {t.Fatalf("unexpected Attempt DTO: %s",araw)}
}
