package application

import (
	"context"
	"encoding/json"
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

type stageOutputAttemptReaderStub struct {
	attempt domain.Attempt
	err     error
	ref     string
	calls   int
}

func (r *stageOutputAttemptReaderStub) GetAttempt(_ context.Context, ref string) (domain.Attempt, error) {
	r.calls++
	r.ref = ref
	return r.attempt, r.err
}

type stageOutputResourceReaderStub struct {
	resources map[string]StageOutputResource
	err       error
	refs      []string
}

func (r *stageOutputResourceReaderStub) GetStageOutputResource(_ context.Context, ref string) (StageOutputResource, error) {
	r.refs = append(r.refs, ref)
	if r.err != nil {
		return StageOutputResource{}, r.err
	}
	resource, ok := r.resources[ref]
	if !ok {
		return StageOutputResource{}, ErrStageOutputUnavailable
	}
	return resource, nil
}

func validStageOutputReadFixture() (
	CreateRunResult,
	domain.Attempt,
	StageOutputResource,
	StageOutputResource,
	StageOutputExpectation,
) {
	outputRef := "stage-output:opaque"
	qualityRef := "stage-output:quality"
	stage := domain.Stage{
		Ref: "stage:quant-sma20", RunRef: "run:one", Code: "quant.sma20",
		Status: domain.StageSucceeded, Version: 2, AttemptNo: 1,
		OutputRef: &outputRef, QualitySummaryRef: &qualityRef,
	}
	run := domain.Run{Ref: "run:one", TemplateVersion: "1.0.0", Status: domain.RunRunning}
	aggregate := CreateRunResult{Run: run, Stages: []domain.Stage{stage}}
	attempt := domain.Attempt{
		Ref: "attempt:one", StageRef: stage.Ref, Number: 1, Status: domain.AttemptSucceeded,
		FencingToken: 1, OutputRef: &outputRef,
	}
	payload := StageOutputResource{
		Ref: outputRef, RunRef: run.Ref, StageRef: stage.Ref, AttemptRef: attempt.Ref,
		Kind: "payload", StageCode: stage.Code, TemplateVersion: run.TemplateVersion,
		RegistryVersion: "1.1.0", RegistryFingerprint: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		OutputType: "quant.sma20_result", SchemaVersion: "1.0.0",
		SchemaRef: "doc/quant/schema/QNT-CTR-005-sma20-stage-output-1.0.0.schema.json",
		Content:   []byte(`{"result_id":"quant_result:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`),
		CreatedAt: "2026-11-24T10:11:12.123456789Z",
	}
	quality := payload
	quality.Ref = qualityRef
	quality.Kind = "quality_summary"
	quality.SchemaRef = "doc/quant/schema/QNT-CTR-005-sma20-quality-summary-1.0.0.schema.json"
	quality.Content = []byte(`{"quality_status":"valid"}`)
	expectation := StageOutputExpectation{
		StageCode: stage.Code, OutputType: payload.OutputType,
		SchemaVersion: payload.SchemaVersion, SchemaRef: payload.SchemaRef,
		QualitySummarySchemaRef: quality.SchemaRef,
		PayloadValidator: func(raw json.RawMessage) error {
			if !json.Valid(raw) {
				return errors.New("payload schema validation failed")
			}
			return nil
		},
		QualitySummaryValidator: func(raw json.RawMessage) error {
			if !json.Valid(raw) {
				return errors.New("quality schema validation failed")
			}
			return nil
		},
	}
	return aggregate, attempt, payload, quality, expectation
}

func newStageOutputReadService(
	aggregate CreateRunResult,
	attempt domain.Attempt,
	payload, quality StageOutputResource,
) (*GetStageOutputForRunStage, *stageOutputResourceReaderStub, *stageOutputAttemptReaderStub, error) {
	outputs := &stageOutputResourceReaderStub{resources: map[string]StageOutputResource{
		payload.Ref: payload, quality.Ref: quality,
	}}
	attempts := &stageOutputAttemptReaderStub{attempt: attempt}
	reader, err := NewGetStageOutputForRunStage(&stageOutputRunReaderStub{result: aggregate}, attempts, outputs)
	return reader, outputs, attempts, err
}

func TestGetStageOutputForRunStageResolvesRunStageAttemptAndBothOutputResources(t *testing.T) {
	aggregate, attempt, payload, quality, expectation := validStageOutputReadFixture()
	reader, outputs, attempts, err := newStageOutputReadService(aggregate, attempt, payload, quality)
	if err != nil {
		t.Fatal(err)
	}
	got, err := reader.Read(context.Background(), aggregate.Run.Ref, expectation)
	if err != nil {
		t.Fatalf("Read() error = %v", err)
	}
	if len(outputs.refs) != 2 || outputs.refs[0] != payload.Ref || outputs.refs[1] != quality.Ref {
		t.Fatalf("resource reads = %#v, want payload then quality references", outputs.refs)
	}
	if attempts.calls != 1 || attempts.ref != attempt.Ref {
		t.Fatalf("Attempt read = calls %d ref %q, want %q", attempts.calls, attempts.ref, attempt.Ref)
	}
	if got.Ref != payload.Ref || got.RunRef != aggregate.Run.Ref || got.StageRef != aggregate.Stages[0].Ref {
		t.Fatalf("returned resource association = %+v", got)
	}
	got.Content[0] = '['
	if payload.Content[0] != '{' {
		t.Fatal("returned output content aliases the storage buffer")
	}
}

func TestGetStageOutputForRunStageRejectsUnfinishedOrUnlinkedStage(t *testing.T) {
	for name, mutate := range map[string]func(*CreateRunResult){
		"running":             func(value *CreateRunResult) { value.Stages[0].Status = domain.StageRunning },
		"missing output ref":  func(value *CreateRunResult) { value.Stages[0].OutputRef = nil },
		"missing quality ref": func(value *CreateRunResult) { value.Stages[0].QualitySummaryRef = nil },
		"wrong run ref":       func(value *CreateRunResult) { value.Stages[0].RunRef = "run:other" },
	} {
		t.Run(name, func(t *testing.T) {
			aggregate, attempt, payload, quality, expectation := validStageOutputReadFixture()
			mutate(&aggregate)
			reader, outputs, _, err := newStageOutputReadService(aggregate, attempt, payload, quality)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.Read(context.Background(), aggregate.Run.Ref, expectation); !errors.Is(err, ErrStageOutputUnavailable) && !errors.Is(err, ErrStageOutputIntegrity) {
				t.Fatalf("Read() error = %v, want unavailable or integrity failure", err)
			}
			if len(outputs.refs) != 0 {
				t.Fatalf("unavailable Stage triggered resource read: %#v", outputs.refs)
			}
		})
	}
}

func TestGetStageOutputForRunStageRejectsPayloadAttemptAndQualityMismatch(t *testing.T) {
	for name, mutate := range map[string]func(*CreateRunResult, *domain.Attempt, *StageOutputResource, *StageOutputResource){
		"wrong attempt stage": func(_ *CreateRunResult, value *domain.Attempt, _, _ *StageOutputResource) {
			value.StageRef = "stage:other"
		},
		"wrong attempt number": func(_ *CreateRunResult, value *domain.Attempt, _, _ *StageOutputResource) { value.Number = 2 },
		"wrong attempt status": func(_ *CreateRunResult, value *domain.Attempt, _, _ *StageOutputResource) {
			value.Status = domain.AttemptDegraded
		},
		"missing fencing token": func(_ *CreateRunResult, value *domain.Attempt, _, _ *StageOutputResource) { value.FencingToken = 0 },
		"attempt output mismatch": func(_ *CreateRunResult, value *domain.Attempt, _, _ *StageOutputResource) {
			other := "stage-output:other"
			value.OutputRef = &other
		},
		"wrong payload run": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.RunRef = "run:other"
		},
		"wrong payload stage": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.StageRef = "stage:other"
		},
		"wrong payload kind": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.Kind = "quality_summary"
		},
		"wrong template": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.TemplateVersion = "2.0.0"
		},
		"wrong output type": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.OutputType = "other"
		},
		"wrong schema": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.SchemaVersion = "2.0.0"
		},
		"wrong schema ref": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.SchemaRef = "other"
		},
		"bad registry fingerprint": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.RegistryFingerprint = "not-a-fingerprint"
		},
		"bad created at": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.CreatedAt = "2026-11-24T10:11:12Z"
		},
		"invalid JSON": func(_ *CreateRunResult, _ *domain.Attempt, value *StageOutputResource, _ *StageOutputResource) {
			value.Content = []byte("{")
		},
		"wrong quality ref": func(value *CreateRunResult, _ *domain.Attempt, _, _ *StageOutputResource) {
			other := "stage-output:other"
			value.Stages[0].QualitySummaryRef = &other
		},
	} {
		t.Run(name, func(t *testing.T) {
			aggregate, attempt, payload, quality, expectation := validStageOutputReadFixture()
			mutate(&aggregate, &attempt, &payload, &quality)
			reader, _, _, err := newStageOutputReadService(aggregate, attempt, payload, quality)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := reader.Read(context.Background(), aggregate.Run.Ref, expectation); !errors.Is(err, ErrStageOutputIntegrity) {
				t.Fatalf("Read() error = %v, want ErrStageOutputIntegrity", err)
			}
		})
	}
}

func TestGetStageOutputForRunStageEnforcesBothSchemaValidators(t *testing.T) {
	for name, configure := range map[string]func(*StageOutputExpectation){
		"payload schema": func(value *StageOutputExpectation) {
			value.PayloadValidator = func(json.RawMessage) error { return errors.New("payload rejected") }
		},
		"quality summary schema": func(value *StageOutputExpectation) {
			value.QualitySummaryValidator = func(json.RawMessage) error { return errors.New("quality rejected") }
		},
	} {
		t.Run(name, func(t *testing.T) {
			aggregate, attempt, payload, quality, expectation := validStageOutputReadFixture()
			configure(&expectation)
			reader, _, _, err := newStageOutputReadService(aggregate, attempt, payload, quality)
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
	aggregate, attempt, payload, quality, expectation := validStageOutputReadFixture()
	duplicate := aggregate.Stages[0]
	duplicate.Ref = "stage:duplicate"
	aggregate.Stages = append(aggregate.Stages, duplicate)
	reader, _, _, err := newStageOutputReadService(aggregate, attempt, payload, quality)
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
