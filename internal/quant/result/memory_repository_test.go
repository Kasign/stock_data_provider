package result

import (
	"context"
	"errors"
	"sync"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/manifest"
)

func TestMemoryRepositorySaveIsIdempotentAndRejectsTamperedIdentity(t *testing.T) {
	repository := NewMemoryRepository()
	first := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	if err := repository.Save(context.Background(), first); err != nil {
		t.Fatalf("first Save() error = %v", err)
	}
	if err := repository.Save(context.Background(), first); err != nil {
		t.Fatalf("idempotent Save() error = %v", err)
	}

	tampered := first
	tampered.Payload = json.RawMessage(`{"metric_code":"momentum_20d","value":0.13}`)
	if err := repository.Save(context.Background(), tampered); !errors.Is(err, manifest.ErrInvalidPayload) {
		t.Fatalf("tampered Save() error = %v, want manifest.ErrInvalidPayload", err)
	}
	if len(repository.records) != 1 {
		t.Fatalf("stored record count = %d, want 1", len(repository.records))
	}
}

func TestMemoryRepositorySaveSnapshotsCallerMemory(t *testing.T) {
	repository := NewMemoryRepository()
	record := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	if err := repository.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	stored := repository.records[record.Manifest.ResultID]
	record.Payload[0] = '['
	record.Manifest.InputSnapshotRefs[0] = "snapshot:mutated"
	if string(stored.Payload) != `{"metric_code":"momentum_20d","value":0.12}` ||
		stored.Manifest.InputSnapshotRefs[0] != "snapshot:001" {
		t.Fatalf("stored record aliases caller memory: %+v", stored)
	}
}

func TestMemoryRepositorySaveHonorsCancellation(t *testing.T) {
	repository := NewMemoryRepository()
	record := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := repository.Save(ctx, record); !errors.Is(err, context.Canceled) {
		t.Fatalf("Save() error = %v, want context.Canceled", err)
	}
	if len(repository.records) != 0 {
		t.Fatalf("cancelled Save() persisted %d records", len(repository.records))
	}
}

func TestMemoryRepositoryConcurrentIdempotentSave(t *testing.T) {
	repository := NewMemoryRepository()
	record := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	const writers = 32
	var wait sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- repository.Save(context.Background(), record)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Save() error = %v", err)
		}
	}
	if len(repository.records) != 1 {
		t.Fatalf("stored record count = %d, want 1", len(repository.records))
	}
}
