package storage

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "sync"
 "testing"
 "time"

 "github.com/go-sql-driver/mysql"
 platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
 "example.com/stock-analysis-flow/internal/researchflow/application"
 "example.com/stock-analysis-flow/internal/researchflow/domain"
 "example.com/stock-analysis-flow/internal/researchflow/template"
)

func deadlineRaceDB(t *testing.T) (context.Context, context.CancelFunc, *sql.DB, *MySQLRunRepository) {
 t.Helper()
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second)
 dsn:=fmt.Sprintf("%s:%s@tcp(%s)/%s?parseTime=true",os.Getenv("PLATFORM_MARIADB_USER"),os.Getenv("PLATFORM_MARIADB_PASSWORD"),os.Getenv("PLATFORM_MARIADB_ADDR"),os.Getenv("PLATFORM_MARIADB_DATABASE"))
 cfg,err:=mysql.ParseDSN(dsn); if err!=nil {cancel();t.Fatal(err)}
 connector,err:=mysql.NewConnector(cfg); if err!=nil {cancel();t.Fatal(err)}
 db,err:=platformstorage.OpenSQL(ctx,connector,platformstorage.DefaultSQLConfig()); if err!=nil {cancel();t.Fatal(err)}
 if err=platformstorage.ApplyMigrations(ctx,db,MigrationNamespace,Migrations()); err!=nil {db.Close();cancel();t.Fatal(err)}
 repo,err:=NewMySQLRunRepository(db); if err!=nil {db.Close();cancel();t.Fatal(err)}
 return ctx,cancel,db,repo
}

func deadlineRaceFixture(t *testing.T,ctx context.Context,repo *MySQLRunRepository,suffix,prefix string,codes ...string)(string,[]string){
 t.Helper(); now:=time.Now().UTC()
 stages:=make([]domain.Stage,0,len(codes)); resolved:=make([]template.ResolvedStage,0,len(codes)); refs:=make([]string,0,len(codes))
 runRef:=prefix+"-"+suffix
 for _,code:=range codes { ref:=prefix+"-"+code+"-"+suffix; refs=append(refs,ref); stages=append(stages,domain.Stage{Ref:ref,RunRef:runRef,Code:code,Status:domain.StageReady,Version:1,Required:true,Dependencies:[]string{},RetryPolicyRef:"retry-v1",DiagnosticRefs:[]string{},ReasonCodes:[]string{}}); resolved=append(resolved,template.ResolvedStage{Definition:template.StageDefinition{Code:code}}) }
 fp:="aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"; tfp:="bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
 rec:=application.CreateRunRecord{Scope:application.IdempotencyScope{CallerRef:prefix,Operation:"CreateRun",Key:suffix},Fingerprint:fp,Run:domain.Run{Ref:runRef,SchemaVersion:"v1",Status:domain.RunCreated,Version:1,SystemAsOf:now.Add(-time.Minute),DeadlineAt:now.Add(5*time.Minute),TemplateRef:prefix+"-template",TemplateVersion:"v1",TemplateFingerprint:tfp,InputSnapshotRefs:[]string{},CorrelationID:prefix+"-"+suffix},Stages:stages,ResolvedTemplate:template.ResolvedTemplate{Ref:prefix+"-template",Version:"v1",Fingerprint:tfp,Stages:resolved},Parameters:json.RawMessage("{}")}
 if _,err:=repo.CreateRunAtomic(ctx,rec);err!=nil{t.Fatal(err)}
 return runRef,refs
}

func TestMariaDBExpireRunDeadlineFencesRunningAttempt(t *testing.T){
 if os.Getenv("PLATFORM_STORAGE_INTEGRATION")!="1"{t.Skip("set PLATFORM_STORAGE_INTEGRATION=1")}
 ctx,cancel,db,repo:=deadlineRaceDB(t);defer cancel();defer db.Close()
 suffix:=fmt.Sprint(time.Now().UnixNano());run,stages:=deadlineRaceFixture(t,ctx,repo,suffix,"deadline-fence","active","pending")
 attempt,replayed,err:=repo.ClaimAttempt(ctx,run,stages[0],"deadline-attempt-"+suffix,"deadline-worker",time.Minute,2,8);if err!=nil||replayed{t.Fatalf("ClaimAttempt replayed=%v err=%v",replayed,err)}
 if _,err=db.ExecContext(ctx,"UPDATE researchflow_runs SET deadline_at=? WHERE run_ref=?",formatResearchflowTimeValue(time.Now().UTC().Add(-time.Minute)),run);err!=nil{t.Fatal(err)}
 runSnapshot,err:=repo.GetRun(ctx,run);if err!=nil{t.Fatal(err)};expectedRunVersion:=runSnapshot.Run.Version
 updated,version,err:=repo.ExpireRunDeadline(ctx,run,expectedRunVersion);if err!=nil||!updated||version!=expectedRunVersion+1{t.Fatalf("ExpireRunDeadline=%v/%d/%v expected version %d",updated,version,err,expectedRunVersion+1)}
 got,err:=repo.GetAttempt(ctx,attempt.Ref);if err!=nil{t.Fatal(err)}
 if got.Status!=domain.AttemptTimedOut||got.FencingToken!=attempt.FencingToken+1||got.LeaseOwner!=""||got.LeaseExpiresAt!=nil{t.Fatalf("Attempt not fenced: %#v",got)}
 for i,ref:=range stages{stage,e:=repo.GetStage(ctx,ref);if e!=nil{t.Fatal(e)};want:=uint64(2);if i==0{want=3};if stage.Status!=domain.StageTimedOut||stage.Version!=want{t.Fatalf("Stage %s=%s/v%d want timed_out/v%d",ref,stage.Status,stage.Version,want)}}
 if err:=repo.CommitAttemptResult(ctx,AttemptResultCommit{RunRef:run,StageRef:stages[0],AttemptRef:attempt.Ref,WorkerID:"deadline-worker",FencingToken:attempt.FencingToken,AttemptStatus:domain.AttemptSucceeded,Stage:domain.Stage{Ref:stages[0],RunRef:run,Code:"active",Status:domain.StageSucceeded,Version:3}});!errors.Is(err,ErrClaimRunUnavailable)&&!errors.Is(err,ErrAttemptLeaseHeld){t.Fatalf("stale result err=%v",err)}
}

func TestMariaDBDeadlineAndUserCancellationHaveSingleWinner(t *testing.T){
 if os.Getenv("PLATFORM_STORAGE_INTEGRATION")!="1"{t.Skip("set PLATFORM_STORAGE_INTEGRATION=1")}
 ctx,cancel,db,repo:=deadlineRaceDB(t);defer cancel();defer db.Close()
 suffix:=fmt.Sprint(time.Now().UnixNano());run,_:=deadlineRaceFixture(t,ctx,repo,suffix,"deadline-cancel","stage")
 if _,err:=db.ExecContext(ctx,"UPDATE researchflow_runs SET deadline_at=? WHERE run_ref=?",formatResearchflowTimeValue(time.Now().UTC().Add(-time.Minute)),run);err!=nil{t.Fatal(err)}
 start:=make(chan struct{});type outcome struct{changed bool;err error};results:=make(chan outcome,2);var wg sync.WaitGroup;wg.Add(2)
 go func(){defer wg.Done();<-start;v,e:=repo.ExpireRunDeadline(ctx,run,1);results<-outcome{v,e}}()
 go func(){defer wg.Done();<-start;v,e:=repo.BeginRunCancellation(ctx,run,1);results<-outcome{v,e}}()
 close(start);wg.Wait();close(results);wins,rejects:=0,0
 for r:=range results{if r.changed&&r.err==nil{wins++;continue};if r.changed||r.err==nil{t.Fatalf("unexpected race result: %#v",r)};if !errors.Is(r.err,ErrClaimRunUnavailable)&&!errors.Is(r.err,application.ErrRunCancellationUnavailable){t.Fatalf("unexpected rejection: %v",r.err)};rejects++}
 if wins!=1||rejects!=1{t.Fatalf("wins=%d rejects=%d",wins,rejects)}
 var status,reason string;var version uint64
 if err:=db.QueryRowContext(ctx,"SELECT status,version,termination_reason FROM researchflow_runs WHERE run_ref=?",run).Scan(&status,&version,&reason);err!=nil{t.Fatal(err)}
 if version!=2{t.Fatalf("Run version=%d want 2",version)}
 switch reason{case "deadline_exceeded":if domain.RunStatus(status)!=domain.RunCreated{t.Fatalf("deadline winner status=%s",status)}
 case "user_cancelled":if domain.RunStatus(status)!=domain.RunCancelling{t.Fatalf("cancellation winner status=%s",status)}
 default:t.Fatalf("unexpected termination reason %q",reason)}
}
