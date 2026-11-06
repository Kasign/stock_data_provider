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
	deadlineRun,updated,err=ApplyDeadline(deadlineRun,stages,now)
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
