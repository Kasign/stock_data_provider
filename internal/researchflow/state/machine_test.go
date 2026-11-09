package state

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

func TestTransitionRejectsIllegalAndTerminalMutation(t *testing.T){
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	if _,err:=TransitionStage(domain.Stage{Status:domain.StagePending},domain.StageSucceeded,now);!errors.Is(err,ErrInvalidTransition){t.Fatalf("expected invalid transition, got %v",err)}
	if _,err:=TransitionRun(domain.Run{Status:domain.RunSucceeded},domain.RunCancelled,now);!errors.Is(err,ErrAlreadyTerminal){t.Fatalf("expected terminal guard, got %v",err)}
}

func TestRetryPreservesAttemptAndEnforcesBudget(t *testing.T){
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	p:=template.RetryPolicy{MaxAttempts:2,RetryableReasonCodes:[]string{"provider_unavailable"},Backoff:template.BackoffPolicy{Algorithm:"exponential_full_jitter_v1",BaseDelayMS:100,MaxDelayMS:1000}}
	stage:=domain.Stage{Ref:"stage-1",Status:domain.StageRunning,AttemptNo:1}
	first:=domain.Attempt{Ref:"attempt-1",StageRef:stage.Ref,Number:1,Status:domain.AttemptRunning}
	waiting,finished,err:=ScheduleRetry(stage,first,p,"provider_unavailable",domain.AttemptFailed,now,now.Add(time.Minute),0.5)
	if err!=nil{t.Fatal(err)}
	if waiting.Status!=domain.StageRetryWait||finished.Status!=domain.AttemptFailed||waiting.RetryAt==nil{t.Fatal("retry transition did not preserve failed attempt")}
	run:=domain.Run{Status:domain.RunRunning,DeadlineAt:now.Add(time.Minute)}
	claimed,next,err:=ClaimRetryStage(run,waiting,p,*waiting.RetryAt,"attempt-2",2)
	if err!=nil{t.Fatal(err)}
	if claimed.Status!=domain.StageRunning||claimed.AttemptNo!=2||claimed.RetryAt!=nil||next.Number!=2||next.Ref==finished.Ref{t.Fatal("retry promotion and next Attempt claim must complete as one operation")}
	if claimed.Version!=waiting.Version+2{t.Fatalf("both retry_wait -> ready and ready -> running transitions must be reflected in the version: before=%d after=%d",waiting.Version,claimed.Version)}
	if _,_,err:=ScheduleRetry(claimed,next,p,"provider_unavailable",domain.AttemptFailed,*waiting.RetryAt,run.DeadlineAt,0.5);!errors.Is(err,ErrRetryBudgetExhausted){t.Fatalf("expected exhausted budget, got %v",err)}
}

func TestCancelAndDeadlineDoNotPretendInflightWorkStopped(t *testing.T){
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	run:=domain.Run{Status:domain.RunRunning,DeadlineAt:now.Add(time.Minute)}
	stages:=[]domain.Stage{{Status:domain.StageRunning},{Status:domain.StageReady}}
	cancelled,updated,err:=CancelRun(run,stages,now)
	if err!=nil{t.Fatal(err)}
	if cancelled.Status!=domain.RunCancelling||updated[0].Status!=domain.StageRunning||updated[1].Status!=domain.StageCancelled{t.Fatal("cancel must await inflight stop confirmation")}
	deadlineRun:=domain.Run{Status:domain.RunRunning,DeadlineAt:now}
	deadlineStages:=[]domain.Stage{{Status:domain.StageRunning},{Status:domain.StageReady}}
	deadlineRun,updated,err=ApplyDeadline(deadlineRun,deadlineStages,now)
	if err!=nil{t.Fatal(err)}
	if deadlineRun.Status!=domain.RunCancelling||updated[0].Status!=domain.StageRunning||updated[1].Status!=domain.StageTimedOut{t.Fatal("deadline must await inflight stop/fencing")}
}

func TestAggregateKeepsQualitySeparateFromActionability(t *testing.T){
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	run:=domain.Run{Status:domain.RunRunning}
	stages:=[]domain.Stage{{Code:"quant",Status:domain.StageDegraded},{Code:"risk",Status:domain.StageSucceeded,RiskDecision:"reject"}}
	policies:=map[string]template.PropagationPolicy{"quant":{OnDegraded:domain.PropagateAccept},"risk":{OnDegraded:domain.PropagateFail}}
	result,err:=AggregateRun(run,stages,nil,policies,map[string]bool{"risk":true},true,now)
	if err!=nil{t.Fatal(err)}
	if result.Status!=domain.RunSucceeded||result.ResultSummary.QualityStatus!="degraded"||result.ResultSummary.ActionabilityStatus!="blocked"{t.Fatalf("unexpected result: %#v",result)}
}

func TestDeadlineAndUserCancellationRemainDistinct(t *testing.T){
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	run:=domain.Run{Status:domain.RunRunning,DeadlineAt:now}
	updated,_,err:=ApplyDeadline(run,[]domain.Stage{{Status:domain.StageReady}},now)
	if err!=nil{t.Fatal(err)}
	if updated.TerminationReason!="run_deadline_exceeded"{t.Fatalf("unexpected reason: %s",updated.TerminationReason)}
}


func TestQueuedRunCannotCancelAnInFlightStage(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	run := domain.Run{Status: domain.RunQueued}
	stages := []domain.Stage{{Status: domain.StageRunning}}
	if _, _, err := CancelRun(run, stages, now); err == nil {
		t.Fatal("queued run with a running stage must fail closed")
	}
}


func TestAggregateHardRiskActionabilityDistinguishesExecutionFailureFromDecision(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	policy := template.PropagationPolicy{
		OnDegraded: domain.PropagateFail, OnSkipped: domain.PropagateFail,
		OnFailed: domain.PropagateFail, OnTimedOut: domain.PropagateFail, OnBlocked: domain.PropagateFail,
	}
	policies := map[string]template.PropagationPolicy{"risk": policy}
	gates := map[string]bool{"risk": true}
	tests := []struct {
		name string
		stage domain.Stage
		wantRun domain.RunStatus
		wantActionability string
	}{
		{name: "execution failed is not evaluated", stage: domain.Stage{Code:"risk", Status:domain.StageFailed, ReasonCodes:[]string{"provider_error"}}, wantRun:domain.RunFailed, wantActionability:"not_evaluated"},
		{name: "business reject is evaluated but blocked", stage: domain.Stage{Code:"risk", Status:domain.StageSucceeded, RiskDecision:"reject"}, wantRun:domain.RunSucceeded, wantActionability:"blocked"},
		{name: "business unknown is evaluated but blocked", stage: domain.Stage{Code:"risk", Status:domain.StageSucceeded, RiskDecision:"unknown"}, wantRun:domain.RunSucceeded, wantActionability:"blocked"},
		{name: "explicit pass is actionable", stage: domain.Stage{Code:"risk", Status:domain.StageSucceeded, RiskDecision:"pass"}, wantRun:domain.RunSucceeded, wantActionability:"actionable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AggregateRun(domain.Run{Status:domain.RunRunning}, []domain.Stage{tt.stage}, nil, policies, gates, true, now)
			if err != nil { t.Fatal(err) }
			if got.Status != tt.wantRun { t.Fatalf("run status = %s, want %s", got.Status, tt.wantRun) }
			if got.ResultSummary.ActionabilityStatus != tt.wantActionability { t.Fatalf("actionability = %s, want %s", got.ResultSummary.ActionabilityStatus, tt.wantActionability) }
		})
	}
}

func TestAggregateOptionalSkippedAndRequiredSkipped(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	policy := template.PropagationPolicy{OnSkipped:domain.PropagateNoImpact}
	optional, err := AggregateRun(domain.Run{Status:domain.RunRunning}, []domain.Stage{{Code:"optional", Status:domain.StageSkipped}}, nil, map[string]template.PropagationPolicy{"optional":policy}, nil, false, now)
	if err != nil { t.Fatal(err) }
	if optional.Status != domain.RunSucceeded { t.Fatalf("optional no-impact skip should succeed, got %s", optional.Status) }
	required, err := AggregateRun(domain.Run{Status:domain.RunRunning}, []domain.Stage{{Code:"required", Required:true, Status:domain.StageSkipped}}, nil, map[string]template.PropagationPolicy{"required":policy}, nil, false, now)
	if err != nil { t.Fatal(err) }
	if required.Status != domain.RunFailed { t.Fatalf("required skipped stage must fail closed, got %s", required.Status) }
}

func TestAggregateProjectsAndDeduplicatesReasonCodes(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	stage := domain.Stage{Code:"market", Status:domain.StageFailed, ReasonCodes:[]string{"provider_unavailable", "provider_unavailable", "invalid_payload"}}
	policy := template.PropagationPolicy{OnFailed:domain.PropagateFail}
	got, err := AggregateRun(domain.Run{Status:domain.RunRunning}, []domain.Stage{stage}, nil, map[string]template.PropagationPolicy{"market":policy}, nil, false, now)
	if err != nil { t.Fatal(err) }
	want := []string{"invalid_payload", "provider_unavailable"}
	if len(got.ResultSummary.ReasonCodes) != len(want) { t.Fatalf("reason codes = %#v", got.ResultSummary.ReasonCodes) }
	for i := range want { if got.ResultSummary.ReasonCodes[i] != want[i] { t.Fatalf("reason codes = %#v, want %#v", got.ResultSummary.ReasonCodes, want) } }
}


func TestAggregateHardRiskPassDoesNotEvaluateFailedRun(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	stages := []domain.Stage{
		{Code:"risk", Status:domain.StageSucceeded, RiskDecision:"pass"},
		{Code:"research", Status:domain.StageFailed, ReasonCodes:[]string{"research_failed"}},
	}
	policies := map[string]template.PropagationPolicy{
		"risk": {OnDegraded:domain.PropagateFail, OnSkipped:domain.PropagateFail, OnFailed:domain.PropagateFail, OnTimedOut:domain.PropagateFail, OnBlocked:domain.PropagateFail},
		"research": {OnFailed:domain.PropagateFail},
	}
	got, err := AggregateRun(domain.Run{Status:domain.RunRunning}, stages, nil, policies, map[string]bool{"risk":true}, true, now)
	if err != nil { t.Fatal(err) }
	if got.Status != domain.RunFailed { t.Fatalf("run status = %s, want failed", got.Status) }
	if got.ResultSummary.ActionabilityStatus != "not_evaluated" { t.Fatalf("actionability = %s, want not_evaluated", got.ResultSummary.ActionabilityStatus) }
}


func TestCancelRunDoesNotMutateInputWhenValidationFails(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	stages := []domain.Stage{{Status:domain.StageReady}, {Status:domain.StageRunning}}
	original := append([]domain.Stage(nil), stages...)
	if _, _, err := CancelRun(domain.Run{Status:domain.RunQueued}, stages, now); err == nil {
		t.Fatal("expected queued run with running stage to be rejected")
	}
	for i := range stages { if stages[i].Status != original[i].Status { t.Fatalf("input stage %d mutated on error", i) } }
}

func TestApplyDeadlineDoesNotMutateInputAndKeepsUserCancellation(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	stages := []domain.Stage{{Status:domain.StageReady}}
	updated, got, err := ApplyDeadline(domain.Run{Status:domain.RunRunning, DeadlineAt:now}, stages, now)
	if err != nil { t.Fatal(err) }
	if updated.Status != domain.RunCancelling || got[0].Status != domain.StageTimedOut { t.Fatal("deadline must time out not-yet-started stages") }
	if stages[0].Status != domain.StageReady { t.Fatal("deadline mutated caller-owned input slice") }
	cancelRun := domain.Run{Status:domain.RunCancelling, TerminationReason:"user_cancelled", DeadlineAt:now}
	unchanged, unchangedStages, err := ApplyDeadline(cancelRun, stages, now)
	if err != nil { t.Fatal(err) }
	if unchanged.TerminationReason != "user_cancelled" || unchanged.Status != domain.RunCancelling || unchangedStages[0].Status != domain.StageReady { t.Fatal("deadline overwrote accepted user cancellation") }
}

func TestOptionalNoImpactSkipDoesNotReduceCoverage(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	stages := []domain.Stage{{Code:"required", Required:true, Status:domain.StageSucceeded}, {Code:"optional", Required:false, Status:domain.StageSkipped}}
	policies := map[string]template.PropagationPolicy{
		"required": {OnSkipped:domain.PropagateFail},
		"optional": {OnSkipped:domain.PropagateNoImpact},
	}
	got, err := AggregateRun(domain.Run{Status:domain.RunRunning}, stages, nil, policies, nil, false, now)
	if err != nil { t.Fatal(err) }
	if got.Status != domain.RunSucceeded || got.ResultSummary.Coverage != 1 { t.Fatalf("no-impact skip reduced coverage: %#v", got.ResultSummary) }
}

func TestAggregateRejectsInvalidHardRiskDecision(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	policy := template.PropagationPolicy{OnDegraded:domain.PropagateFail, OnSkipped:domain.PropagateFail, OnFailed:domain.PropagateFail, OnTimedOut:domain.PropagateFail, OnBlocked:domain.PropagateFail}
	_, err := AggregateRun(domain.Run{Status:domain.RunRunning}, []domain.Stage{{Code:"risk", Status:domain.StageSucceeded, RiskDecision:""}}, nil, map[string]template.PropagationPolicy{"risk":policy}, map[string]bool{"risk":true}, true, now)
	if err == nil { t.Fatal("empty hard risk decision was accepted") }
}

func TestFinalizeCancellationPersistsTerminalSummary(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	run := domain.Run{Status:domain.RunCancelling, TerminationReason:"user_cancelled"}
	stages := []domain.Stage{
		{Code:"done", Status:domain.StageSucceeded},
		{Code:"pending", Status:domain.StageCancelled, ReasonCodes:[]string{"user_cancelled"}},
	}
	got, err := FinalizeCancellation(run, stages, nil, now)
	if err != nil { t.Fatal(err) }
	if got.Status != domain.RunCancelled || got.ResultSummary == nil { t.Fatal("cancelled terminal Run must have a fixed result summary") }
	if got.ResultSummary.StageOutcomes["done"] != domain.StageSucceeded || got.ResultSummary.StageOutcomes["pending"] != domain.StageCancelled { t.Fatal("terminal summary omitted stage outcomes") }
	if got.ResultSummary.ActionabilityStatus != "not_evaluated" { t.Fatal("cancelled Run must not be actionable") }
}


func TestQueuedCancellationCreatesTerminalSummary(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	run := domain.Run{Status:domain.RunQueued}
	stages := []domain.Stage{{Code:"market", Status:domain.StageReady}}
	got, updated, err := CancelRun(run, stages, now)
	if err != nil { t.Fatal(err) }
	if got.Status != domain.RunCancelled || got.ResultSummary == nil { t.Fatal("queued cancellation must persist terminal summary") }
	if got.ResultSummary.StageOutcomes["market"] != domain.StageCancelled || updated[0].Status != domain.StageCancelled { t.Fatal("cancelled stage missing from terminal summary") }
	if stages[0].Status != domain.StageReady { t.Fatal("CancelRun mutated caller-owned input slice") }
}


func TestTransitionRunCannotBypassTerminalFinalization(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	run := domain.Run{Status: domain.RunRunning}
	if _, err := TransitionRun(run, domain.RunSucceeded, now); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("terminal transition must require a finalizer, got %v", err)
	}
}

func TestWorkerClaimIsSeparateFromRetryPromotion(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	run := domain.Run{Status: domain.RunRunning, DeadlineAt: now.Add(time.Minute)}
	stage := domain.Stage{Ref: "stage-1", Status: domain.StageReady, AttemptNo: 1}
	claimed, attempt, err := ClaimReadyStage(run, stage, now, "attempt-2", 2)
	if err != nil { t.Fatal(err) }
	if claimed.Status != domain.StageRunning || claimed.AttemptNo != 2 || attempt.Number != 2 {
		t.Fatalf("unexpected claim result: stage=%#v attempt=%#v", claimed, attempt)
	}
	if _, _, err := ClaimReadyStage(domain.Run{Status: domain.RunCancelling, DeadlineAt: now.Add(time.Minute)}, stage, now, "attempt-3", 3); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("claim during cancellation must be rejected, got %v", err)
	}
	if _, _, err := ClaimReadyStage(domain.Run{Status: domain.RunRunning, DeadlineAt: now}, stage, now, "attempt-3", 3); !errors.Is(err, ErrDeadlineExceeded) {
		t.Fatalf("claim at deadline must be rejected, got %v", err)
	}
}


func TestScheduleRetryDoesNotMutateCallerReasonCodeBackingArray(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	reasons := make([]string, 1, 3)
	reasons[0] = "previous_reason"
	stage := domain.Stage{Ref: "stage-1", Status: domain.StageRunning, AttemptNo: 1, ReasonCodes: reasons}
	attempt := domain.Attempt{Ref: "attempt-1", StageRef: stage.Ref, Number: 1, Status: domain.AttemptRunning}
	updated, _, err := ScheduleRetry(stage, attempt, template.RetryPolicy{
		MaxAttempts: 2, RetryableReasonCodes: []string{"provider_unavailable"},
		Backoff: template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
	}, "provider_unavailable", domain.AttemptFailed, now, now.Add(time.Minute), 0.5)
	if err != nil { t.Fatal(err) }
	if got := reasons[:2]; got[1] != "" {
		t.Fatalf("ScheduleRetry mutated caller-owned backing array: %#v", got)
	}
	if len(updated.ReasonCodes) != 2 || updated.ReasonCodes[1] != "provider_unavailable" {
		t.Fatalf("updated reason codes missing retry reason: %#v", updated.ReasonCodes)
	}
}

func TestWorkerClaimRequiresPositiveFencingToken(t *testing.T) {
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	run:=domain.Run{Status:domain.RunRunning,DeadlineAt:now.Add(time.Minute)}
	stage:=domain.Stage{Ref:"stage-1",Status:domain.StageReady}
	if _,_,err:=ClaimReadyStage(run,stage,now,"attempt-1",0);!errors.Is(err,ErrInvalidTransition){t.Fatalf("zero fencing token must be rejected, got %v",err)}
	_,attempt,err:=ClaimReadyStage(run,stage,now,"attempt-1",1)
	if err!=nil{t.Fatal(err)}
	if attempt.FencingToken!=1{t.Fatalf("fencing token = %d, want 1",attempt.FencingToken)}
}

func TestRunAggregationAndCancellationPreserveExcludedStageEvidence(t *testing.T) {
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	excluded:=[]domain.ExcludedStage{{StageCode:"optional_news",RuleVersion:"v2",ReasonCode:"market_closed"}}
	policy:=template.PropagationPolicy{OnFailed:domain.PropagateFail}
	run:=domain.Run{Status:domain.RunRunning,ExcludedStages:excluded}
	aggregated,err:=AggregateRun(run,[]domain.Stage{{Code:"quote",Status:domain.StageSucceeded}},nil,map[string]template.PropagationPolicy{"quote":policy},nil,false,now)
	if err!=nil{t.Fatal(err)}
	if len(aggregated.ResultSummary.ExcludedStages)!=1||aggregated.ResultSummary.ExcludedStages[0].ReasonCode!="market_closed"{
		t.Fatalf("Run aggregation lost applicability exclusions: %#v",aggregated.ResultSummary.ExcludedStages)
	}
	queued:=domain.Run{Status:domain.RunQueued,ExcludedStages:excluded}
	cancelled,_,err:=CancelRun(queued,[]domain.Stage{{Code:"quote",Status:domain.StageReady}},now)
	if err!=nil{t.Fatal(err)}
	if cancelled.ResultSummary==nil||len(cancelled.ResultSummary.ExcludedStages)!=1{
		t.Fatalf("cancellation summary lost applicability exclusions: %#v",cancelled.ResultSummary)
	}
}

func TestRunAndStageTransitionTables(t *testing.T) {
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	runCases:=[]struct{name string;from,next domain.RunStatus;wantErr bool}{
		{"created_to_queued",domain.RunCreated,domain.RunQueued,false},
		{"queued_to_running",domain.RunQueued,domain.RunRunning,false},
		{"queued_to_cancelling",domain.RunQueued,domain.RunCancelling,false},
		{"running_to_cancelling",domain.RunRunning,domain.RunCancelling,false},
		{"created_to_running",domain.RunCreated,domain.RunRunning,true},
		{"running_to_succeeded_requires_finalizer",domain.RunRunning,domain.RunSucceeded,true},
		{"cancelling_to_running",domain.RunCancelling,domain.RunRunning,true},
		{"terminal_run_is_immutable",domain.RunSucceeded,domain.RunCancelling,true},
	}
	for _,tc:=range runCases{t.Run(tc.name,func(t *testing.T){
		_,err:=TransitionRun(domain.Run{Status:tc.from},tc.next,now)
		if (err!=nil)!=tc.wantErr{t.Fatalf("TransitionRun(%s -> %s) error=%v wantErr=%v",tc.from,tc.next,err,tc.wantErr)}
	})}
	stageCases:=[]struct{name string;from,next domain.StageStatus;wantErr bool}{
		{"pending_to_ready",domain.StagePending,domain.StageReady,false},
		{"pending_to_blocked",domain.StagePending,domain.StageBlocked,false},
		{"ready_to_running",domain.StageReady,domain.StageRunning,false},
		{"running_to_retry_wait",domain.StageRunning,domain.StageRetryWait,false},
		{"running_to_degraded",domain.StageRunning,domain.StageDegraded,false},
		{"retry_wait_to_ready",domain.StageRetryWait,domain.StageReady,false},
		{"pending_to_succeeded",domain.StagePending,domain.StageSucceeded,true},
		{"running_to_skipped",domain.StageRunning,domain.StageSkipped,true},
		{"terminal_stage_is_immutable",domain.StageFailed,domain.StageReady,true},
	}
	for _,tc:=range stageCases{t.Run(tc.name,func(t *testing.T){
		_,err:=TransitionStage(domain.Stage{Status:tc.from},tc.next,now)
		if (err!=nil)!=tc.wantErr{t.Fatalf("TransitionStage(%s -> %s) error=%v wantErr=%v",tc.from,tc.next,err,tc.wantErr)}
	})}
}

func TestFinalizeDeadlineRequiresAllWorkStoppedAndUsesPropagation(t *testing.T) {
	now:=time.Date(2026,11,6,10,0,0,0,time.UTC)
	run:=domain.Run{Status:domain.RunCancelling,TerminationReason:"run_deadline_exceeded"}
	policies:=map[string]template.PropagationPolicy{
		"required":{OnTimedOut:domain.PropagateFail},
		"optional":{OnTimedOut:domain.PropagatePartial},
	}
	stages:=[]domain.Stage{
		{Code:"required",Required:true,Status:domain.StageTimedOut,ReasonCodes:[]string{"run_deadline_exceeded"}},
		{Code:"optional",Required:false,Status:domain.StageTimedOut,ReasonCodes:[]string{"run_deadline_exceeded"}},
	}
	got,err:=FinalizeDeadline(run,stages,nil,policies,nil,false,now)
	if err!=nil{t.Fatal(err)}
	if got.Status!=domain.RunFailed||got.ResultSummary==nil||got.ResultSummary.ActionabilityStatus!="not_evaluated"{
		t.Fatalf("deadline propagation/finalization mismatch: %#v",got)
	}
	runningAttempt:=domain.Attempt{Status:domain.AttemptRunning}
	if _,err:=FinalizeDeadline(run,stages,[]domain.Attempt{runningAttempt},policies,nil,false,now);err==nil{
		t.Fatal("deadline finalizer accepted a still-running Attempt")
	}
}


func TestApplyDeadlinePreservesCompletedRunForNormalAggregation(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	run := domain.Run{Status: domain.RunRunning, DeadlineAt: now}
	stages := []domain.Stage{
		{Code: "market", Status: domain.StageSucceeded},
		{Code: "research", Status: domain.StageSucceeded},
	}
	updatedRun, updatedStages, err := ApplyDeadline(run, stages, now)
	if err != nil {
		t.Fatal(err)
	}
	if updatedRun.Status != domain.RunRunning || updatedRun.TerminationReason != "" {
		t.Fatalf("deadline changed an already-complete Run: %#v", updatedRun)
	}
	for i := range stages {
		if updatedStages[i].Status != stages[i].Status {
			t.Fatalf("deadline changed completed Stage %q from %s to %s", stages[i].Code, stages[i].Status, updatedStages[i].Status)
		}
	}
	policies := map[string]template.PropagationPolicy{
		"market": {OnFailed: domain.PropagateFail},
		"research": {OnFailed: domain.PropagateFail},
	}
	finalized, err := AggregateRun(updatedRun, updatedStages, nil, policies, nil, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if finalized.Status != domain.RunSucceeded || finalized.ResultSummary == nil {
		t.Fatalf("completed Run did not retain normal successful finalization: %#v", finalized)
	}
}


func TestClaimRetryStageRejectsEarlyOrExhaustedRetry(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	policy := template.RetryPolicy{MaxAttempts: 2}
	retryAt := now.Add(time.Second)
	stage := domain.Stage{Ref: "stage-1", Status: domain.StageRetryWait, Version: 3, AttemptNo: 1, RetryAt: &retryAt}
	run := domain.Run{Status: domain.RunRunning, DeadlineAt: now.Add(time.Minute)}
	if _, _, err := ClaimRetryStage(run, stage, policy, now, "attempt-2", 2); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("retry must not be claimed before retry_at: %v", err)
	}
	if _, _, err := ClaimRetryStage(run, stage, policy, now.Add(time.Second), "attempt-2", 2); err != nil {
		t.Fatalf("eligible retry should be claimable: %v", err)
	}
	stage.AttemptNo = 2
	if _, _, err := ClaimRetryStage(run, stage, policy, now.Add(time.Second), "attempt-3", 3); !errors.Is(err, ErrRetryBudgetExhausted) {
		t.Fatalf("expected exhausted retry budget, got %v", err)
	}
}
