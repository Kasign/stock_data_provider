package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type stageOutputRunReaderStub struct {
	result CreateRunResult
	err    error
	calls  int
}

func (r *stageOutputRunReaderStub) GetRun(context.Context, string) (CreateRunResult, error) {
	r.calls++
	return r.result, r.err
}

type stageOutputResourceReaderStub struct {
	resource StageOutputResource
	err      error
	ref      string
	calls    int
}

func (r *stageOutputResourceReaderStub) GetStageOutputResource(_ context.Context, ref string) (StageOutputResource, error) {
	r.calls++
	r.ref = ref
	return r.resource, r.err
}

func validStageOutputReadFixture() (CreateRunResult, StageOutputResource, StageOutputExpectation) {
	outputRef := "stage-output:opaque"
	qualityRef := "stage-output:quality"
	stage := domain.Stage{
		Ref: "stage:quant-sma20", RunRef: "run:one", Code: "quant.sma20",
		Status: domain.StageSucceeded, Version: 2, AttemptNo: 1,
		OutputRef: &outputRef, QualitySummaryRef: &qualityRef,
	}
	run := domain.Run{Ref: "run:one", TemplateVersion: "1.0.0", Status: domain.RunRunning}
	aggregate := CreateRunResult{Run: run, Stages: []domain.Stage{stage}}
	resource := StageOutputResource{
		Ref: outputRef, RunRef: run.Ref, StageRef: stage.Ref, AttemptRef: "attempt:one",
		Kind: "payload", StageCode: stage.Code, TemplateVersion: run.TemplateVersion,
		RegistryVersion: "1.1.0", RegistryFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		OutputType: "quant.sma20.output", SchemaVersion: "1.0.0",
		SchemaRef: "doc/quant/schema/sma20.schema.json",
		Content: []byte(`{"result_id":"quant_result:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
		CreatedAt: "2026-11-24T10:11:12.123456789Z",
	}
	expectation := StageOutputExpectation{
		StageCode: stage.Code, OutputType: resource.OutputType,
		SchemaVersion: resource.SchemaVersion, SchemaRef: resource.SchemaRef,
	}
	return aggregate, resource, expectation
}

func TestGetStageOutputForRunStageResolvesOnlyPersistedRunStageAssociation(t *testing.T) {
	aggregate, resource, expectation := validStageOutputReadFixture()
	runs := &stageOutputRunReaderStub{result: aggregate}
	outputs := &stageOutputResourceReaderStub{resource: resource}
	reader, err := NewGetStageOutputForRunStage(runs, outputs)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.Read(context.Background(), aggregate.Run.Ref, expectation)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if outputs.calls != 1 || outputs.ref != *aggregate.Stages[0].OutputRef {
		t.Fatalf("resource read did not use persisted Stage.OutputRef: calls=%d ref=%q", outputs.calls, outputs.ref)
	}
	if got.Ref != resource.Ref || got.RunRef != aggregate.Run.Ref || got.StageRef != aggregate.Stages[0].Ref {
		t.Fatalf("returned resource association = %+v", got)
	}
	got.Content[0] = '['
	if resource.Content[0] != '{' {
		t.Fatal("returned output content aliases the storage buffer")
	}
}

func TestGetStageOutputForRunStageRejectsUnfinishedOrUnlinkedStage(t *testing.T) {
	for name, mutate := range map[string]func(*CreateRunResult){
		"running": func(value *CreateRunResult) { value.Stages[0].Status = domain.StageRunning },
		"missing output ref": func(value *CreateRunResult) { value.Stages[0].OutputRef = nil },
		"missing quality ref": func(value *CreateRunResult) { value.Stages[0].QualitySummaryRef = nil },
		"wrong run ref": func(value *CreateRunResult) { value.Stages[0].RunRef = "run:other" },
	} {
		t.Run(name, func(t *testing.T) {
			aggregate, resource, expectation := validStageOutputReadFixture()
			mutate(&aggregate)
			runs := &stageOutputRunReaderStub{result: aggregate}
			outputs := &stageOutputResourceReaderStub{resource: resource}
			reader, err := NewGetStageOutputForRunStage(runs, outputs)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.Read(context.Background(), aggregate.Run.Ref, expectation); !errors.Is(err, ErrStageOutputUnavailable) && !errors.Is(err, ErrStageOutputIntegrity) {
				t.Fatalf("Read() error = %v, want unavailable or integrity failure", err)
			}
			if outputs.calls != 0 {
				t.Fatalf("unavailable Stage triggered resource read: %d", outputs.calls)
			}
		})
	}
}

func TestGetStageOutputForRunStageRejectsResourceMetadataMismatch(t *testing.T) {
	for name, mutate := range map[string]func(*StageOutputResource){
		"wrong run": func(value *StageOutputResource) { value.RunRef = "run:other" },
		"wrong stage": func(value *StageOutputResource) { value.StageRef = "stage:other" },
		"wrong attempt": func(value *StageOutputResource) { value.AttemptRef = "" },
		"wrong kind": func(value *StageOutputResource) { value.Kind = "quality_summary" },
		"wrong template": func(value *StageOutputResource) { value.TemplateVersion = "2.0.0" },
		"wrong output type": func(value *StageOutputResource) { value.OutputType = "other" },
		"wrong schema": func(value *StageOutputResource) { value.SchemaVersion = "2.0.0" },
		"wrong schema ref": func(value *StageOutputResource) { value.SchemaRef = "other" },
		"bad registry fingerprint": func(value *StageOutputResource) { value.RegistryFingerprint = "not-a-fingerprint" },
		"bad created at": func(value *StageOutputResource) { value.CreatedAt = "2026-11-24T10:11:12Z" },
		"invalid JSON": func(value *StageOutputResource) { value.Content = []byte("{") },
	} {
		t.Run(name, func(t *testing.T) {
			aggregate, resource, expectation := validStageOutputReadFixture()
			mutate(&resource)
			reader, err := NewGetStageOutputForRunStage(
				&stageOutputRunReaderStub{result: aggregate},
				&stageOutputResourceReaderStub{resource: resource},
			)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.Read(context.Background(), aggregate.Run.Ref, expectation); !errors.Is(err, ErrStageOutputIntegrity) {
				t.Fatalf("Read() error = %v, want ErrStageOutputIntegrity", err)
			}
		})
	}
}

func TestGetStageOutputForRunStageRejectsDuplicateStageCodeAndHonorsCancellation(t *testing.T) {
	aggregate, resource, expectation := validStageOutputReadFixture()
	duplicate := aggregate.Stages[0]
	duplicate.Ref = "stage:duplicate"
	aggregate.Stages = append(aggregate.Stages, duplicate)
	reader, err := NewGetStageOutputForRunStage(
		&stageOutputRunReaderStub{result: aggregate},
		&stageOutputResourceReaderStub{resource: resource},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reader.Read(context.Background(), aggregate.Run.Ref, expectation); !errors.Is(err, ErrStageOutputIntegrity) {
		t.Fatalf("duplicate Stage code error = %v, want integrity failure", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := reader.Read(ctx, aggregate.Run.Ref, expectation); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Read() error = %v, want context.Canceled", err)
	}
}
