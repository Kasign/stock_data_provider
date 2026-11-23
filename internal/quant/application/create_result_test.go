package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

type captureResultRepository struct {
	record result.Record
	calls  int
	err    error
}

func (repository *captureResultRepository) Save(_ context.Context, record result.Record) error {
	repository.calls++
	repository.record = record
	return repository.err
}

func validTrendCreationInput() (manifest.Input, result.TrendPayload) {
	asOf := "2026-11-23T00:00:00Z"
	payload, err := result.NewTrendPayload(trend.Result{
		TrendCode:        "trend.daily",
		Direction:        trend.DirectionBullish,
		Phase:            trend.PhaseEmerging,
		Period:           "1d",
		EvidenceMetrics:  []metric.Metric{},
		ConflictRefs:     []string{},
		QualityStatus:    trend.QualityValid,
		SystemAsOf:       asOf,
		AlgorithmVersion: "trend.v1",
	})
	if err != nil {
		panic(err)
	}
	return manifest.Input{
		ResultType: "trend", Scope: "stock", SecurityID: "600000.SH",
		Period:               "1d",
		QualityStatus:        "valid",
		ResultStatus:         "completed",
		SchemaVersion:        result.EnvelopeSchemaVersion,
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "trend.v1",
		ParameterFingerprint: "sha256:parameters",
		SystemAsOf:           time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"snapshot:daily:600000.SH:2026-11-23"},
	}, payload
}

func TestCreateResultPersistsTypedPayloadAndReturnsManifest(t *testing.T) {
	input, payload := validTrendCreationInput()
	repository := &captureResultRepository{}
	usecase, err := newResultPersister(repository)
	if err != nil {
		t.Fatal(err)
	}
	created, err := usecase.persist(context.Background(), input, payload)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if repository.calls != 1 {
		t.Fatalf("Save calls = %d, want 1", repository.calls)
	}
	if created.ResultID == "" || repository.record.Manifest.ResultID != created.ResultID {
		t.Fatalf("returned manifest does not match persisted record: returned=%q stored=%q", created.ResultID, repository.record.Manifest.ResultID)
	}
	if err := repository.record.Validate(); err != nil {
		t.Fatalf("persisted record is invalid: %v", err)
	}
}

func TestCreateResultRejectsPayloadDiscriminatorMismatchBeforeSave(t *testing.T) {
	input, payload := validTrendCreationInput()
	input.ResultType = "risk"
	repository := &captureResultRepository{}
	usecase, err := newResultPersister(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := usecase.persist(context.Background(), input, payload); !errors.Is(err, result.ErrInvalidEnvelope) {
		t.Fatalf("Execute() error = %v, want ErrInvalidEnvelope", err)
	}
	if repository.calls != 0 {
		t.Fatalf("Save called %d times for invalid payload", repository.calls)
	}
}

func TestCreateResultRejectsPayloadMetadataMismatchBeforeSave(t *testing.T) {
	input, payload := validTrendCreationInput()
	input.Period = "5d"
	repository := &captureResultRepository{}
	usecase, err := newResultPersister(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := usecase.persist(context.Background(), input, payload); !errors.Is(err, result.ErrInvalidEnvelope) {
		t.Fatalf("Execute() error = %v, want ErrInvalidEnvelope", err)
	}
	if repository.calls != 0 {
		t.Fatalf("Save called %d times for mismatched payload metadata", repository.calls)
	}
}

func TestCreateResultDoesNotSaveWhenContextCancelled(t *testing.T) {
	input, payload := validTrendCreationInput()
	repository := &captureResultRepository{}
	usecase, err := newResultPersister(repository)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := usecase.persist(ctx, input, payload); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if repository.calls != 0 {
		t.Fatalf("Save called %d times for cancelled request", repository.calls)
	}
}

func TestCreateResultWrapsPersistenceFailure(t *testing.T) {
	input, payload := validTrendCreationInput()
	repository := &captureResultRepository{err: errors.New("database unavailable")}
	usecase, err := newResultPersister(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := usecase.persist(context.Background(), input, payload); !errors.Is(err, ErrResultPersistenceFailed) {
		t.Fatalf("Execute() error = %v, want ErrResultPersistenceFailed", err)
	}
}
