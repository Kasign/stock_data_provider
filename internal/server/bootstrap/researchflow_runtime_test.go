package bootstrap

import (
	"context"
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
)

func TestResearchflowRuntimeLaunchTaskBoundsAndDeduplicatesWork(t *testing.T) {
	component := &researchflowRuntimeComponent{
		taskSlots: make(chan struct{}, 2),
		inFlight:  make(map[string]struct{}),
	}
	ctx := context.Background()
	started := make(chan struct{}, 2)
	release := make(chan struct{})

	if !component.launchTask(ctx, "run:one", func() {
		started <- struct{}{}
		<-release
	}) {
		t.Fatal("first task was not launched")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}
	if component.launchTask(ctx, "run:one", func() {}) {
		t.Fatal("duplicate task key was launched while a slot was available")
	}
	if !component.launchTask(ctx, "run:two", func() {
		started <- struct{}{}
		<-release
	}) {
		t.Fatal("second task within the local concurrency bound was not launched")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("second task did not start")
	}
	if component.launchTask(ctx, "run:three", func() {}) {
		t.Fatal("task beyond local concurrency bound was launched")
	}

	close(release)
	done := make(chan struct{})
	go func() {
		component.tasks.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("launched task did not finish")
	}
	if !component.launchTask(ctx, "run:two", func() {}) {
		t.Fatal("available local slot was not reused")
	}
	component.tasks.Wait()
}

func TestResearchflowRuntimeLaunchTaskRejectsCancelledContext(t *testing.T) {
	component := &researchflowRuntimeComponent{
		taskSlots: make(chan struct{}, 1),
		inFlight:  make(map[string]struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if component.launchTask(ctx, "run:one", func() {}) {
		t.Fatal("task was launched with a cancelled context")
	}
	if len(component.taskSlots) != 0 {
		t.Fatal("cancelled task consumed a local worker slot")
	}
}

type runtimeStageExecutorFunc func(context.Context, researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error)

func (f runtimeStageExecutorFunc) ExecuteStage(ctx context.Context, request researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error) {
	return f(ctx, request)
}

func TestPrincipalStageExecutorInjectsQuantIdentityOnlyForQuantStage(t *testing.T) {
	secret := platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "0123456789abcdef0123456789abcdef", true
	}).Secret("TOKEN")
	principal, err := platformsecurity.PrincipalFromServiceCredential(platformsecurity.ServiceCredential{
		Name: "quant", Secret: secret, Scopes: []string{platformsecurity.ScopeDataRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	executor := principalStageExecutor{
		quantPrincipal: &principal,
		next: runtimeStageExecutorFunc(func(ctx context.Context, request researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error) {
			calls++
			got, ok := platformsecurity.PrincipalFromContextValue(ctx)
			if isQuantStageCode(request.Stage.Code) {
				if !ok || got.Subject != "quant" || !containsServiceScope(got.Scopes, platformsecurity.ScopeDataRead) {
					t.Fatalf("Quant Stage %s missing its Data-scoped Principal: %+v, ok=%t", request.Stage.Code, got, ok)
				}
			} else if ok {
				t.Fatalf("non-Quant Stage unexpectedly inherited service Principal: %+v", got)
			}
			return researchflowapplication.StageExecutionResult{}, nil
		}),
	}
	stageCodes := []string{
		"news.search_candidates",
		quantstage.SMA20StageCode,
		quantstage.FactorStageCode,
		quantstage.TrendStageCode,
		quantstage.RiskStageCode,
	}
	for _, code := range stageCodes {
		if _, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
			Stage: researchflowdomain.Stage{Code: code},
		}); err != nil {
			t.Fatal(err)
		}
	}
	if calls != len(stageCodes) {
		t.Fatalf("Stage Executor calls = %d, want %d", calls, len(stageCodes))
	}
}

func TestPrincipalStageExecutorFailsClosedWhenQuantIdentityMissing(t *testing.T) {
	executor := principalStageExecutor{
		next: runtimeStageExecutorFunc(func(context.Context, researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error) {
			t.Fatal("Quant Stage must not execute without its Data service identity")
			return researchflowapplication.StageExecutionResult{}, nil
		}),
	}
	// 每个消费 Data Snapshot 的 Quant Stage 都必须 fail closed：
	// 只覆盖 SMA20 会让 quant.factor/quant.trend 在生产 Worker 中被 Data 拒绝。
	for _, code := range []string{
		quantstage.SMA20StageCode,
		quantstage.FactorStageCode,
		quantstage.TrendStageCode,
		quantstage.RiskStageCode,
	} {
		if _, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
			Stage: researchflowdomain.Stage{Code: code},
		}); err == nil {
			t.Fatalf("Quant Stage %s executed without a Quant service Principal", code)
		}
	}
}
