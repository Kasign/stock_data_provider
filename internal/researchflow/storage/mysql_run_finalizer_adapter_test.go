package storage

import (
	"context"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

func TestFinalizeRunResultRejectsInvalidNormalTerminalState(t *testing.T) {
	r:=&MySQLRunRepository{}
	record:=application.RunFinalizationRecord{RunRef:"run",ExpectedVersion:1,ResultSummaryRef:"summary",SummarySchemaVersion:"1",OutboxRef:"outbox",TerminalStatus:domain.RunCancelled}
	if _,err:=r.FinalizeRunResult(context.Background(),record);err==nil{t.Fatal("expected cancelled result to be rejected")}
	record.TerminalStatus=domain.RunSucceeded
	record.TerminationReason="deadline_exceeded"
	if _,err:=r.FinalizeRunResult(context.Background(),record);err==nil{t.Fatal("expected deadline reason to be rejected")}
}
