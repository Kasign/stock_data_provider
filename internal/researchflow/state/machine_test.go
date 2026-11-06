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
	ready,next,err:=PromoteRetry(run,waiting,*waiting.RetryAt,"attempt-2")
	if err!=nil{t.Fatal(err)}
	if ready.AttemptNo!=2||next.Number!=2||next.Ref==finished.Ref{t.Fatal("retry did not append a new attempt")}
	if _,_,err:=ScheduleRetry(ready,next,p,"provider_unavailable",domain.AttemptFailed,*waiting.RetryAt,run.DeadlineAt,0.5);!errors.Is(err,ErrRetryBudgetExhausted){t.Fatalf("expected exhausted budget, got %v",err)}
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
