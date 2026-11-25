package storage

import (
	"context"
	"errors"
	"testing"
	"time"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
)

func TestMySQLAuthorizedResultReaderUsesCurrentPolicyAndStoredRunAssociation(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	policyStore, err := NewMySQLResultAccessPolicyStore(db)
	if err != nil {
		t.Fatal(err)
	}
	record := quantStorageRecord(t, `{"direction":"up","strength":0.7}`)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM quant_results WHERE result_id = ?", record.Manifest.ResultID)
	})

	state, err := policyStore.CurrentResultAccessPolicy(ctx)
	if err != nil {
		t.Fatal(err)
	}
	changedPolicy := false
	if !state.Enabled {
		state, err = policyStore.SetResultAccessPolicyEnabled(ctx, "owner", state.Version, true)
		if err != nil {
			t.Fatal(err)
		}
		changedPolicy = true
	}
	if changedPolicy {
		t.Cleanup(func() {
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cleanupCancel()
			current, currentErr := policyStore.CurrentResultAccessPolicy(cleanupCtx)
			if currentErr == nil && current.Enabled {
				_, _ = policyStore.SetResultAccessPolicyEnabled(cleanupCtx, "owner", current.Version, false)
			}
		})
	}

	if err := repository.Save(ctx, record); err != nil {
		t.Fatal(err)
	}
	association := &resultAssociationVerifierStub{}
	reader, err := NewMySQLAuthorizedResultReader(policyStore, repository, association)
	if err != nil {
		t.Fatal(err)
	}
	request := quantapplication.ReadRequest{
		RunRef: "run:researchflow:quant-result-reader-test",
		SubjectRef: "service:researchflow",
		ResourceRef: record.Manifest.ResultID,
		PurposeCode: "quant_analysis",
		ContentLevel: "full",
		At: time.Date(2026, 11, 25, 2, 0, 0, 0, time.UTC),
	}
	got, err := reader.ReadAuthorized(ctx, request)
	if err != nil {
		t.Fatalf("ReadAuthorized() error = %v", err)
	}
	if got.Record.Manifest.ResultID != record.Manifest.ResultID ||
		got.AuthorizationRef.PolicyVersion != state.Snapshot.SnapshotRef ||
		association.calls != 1 || association.runRef != request.RunRef ||
		association.resultID != request.ResourceRef {
		t.Fatalf("authorized result did not retain current policy and Run association: %+v", got)
	}

	association.err = quantapplication.ErrResultAssociationNotFound
	if _, err := reader.ReadAuthorized(ctx, request); !errors.Is(err, quantapplication.ErrAccessDenied) {
		t.Fatalf("unassociated result read = %v, want ErrAccessDenied", err)
	}
}
