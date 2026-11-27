package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
)

type limitPoolReaderStub struct {
	up, down *model.LimitPoolSnapshot
}

func (s limitPoolReaderStub) FetchFullLimitPool(_ context.Context, poolType, _ string) (*model.LimitPoolSnapshot, error) {
	switch poolType {
	case "up":
		return s.up, nil
	case "down":
		return s.down, nil
	default:
		return nil, errors.New("unsupported pool type")
	}
}

type limitPoolSnapshotRepositoryStub struct {
	saved   app.LimitPoolFactsSnapshot
	hasSave bool
}

func (s *limitPoolSnapshotRepositoryStub) SaveLimitPoolFactsSnapshot(_ context.Context, snapshot app.LimitPoolFactsSnapshot) error {
	s.saved, s.hasSave = snapshot, true
	return nil
}

func (s *limitPoolSnapshotRepositoryStub) GetLimitPoolFactsSnapshot(_ context.Context, ref string) (app.LimitPoolFactsSnapshot, error) {
	if !s.hasSave || s.saved.Manifest.SnapshotRef != ref {
		return app.LimitPoolFactsSnapshot{}, snapshotport.ErrLimitPoolFactsSnapshotNotFound
	}
	return s.saved, nil
}

func (s *limitPoolSnapshotRepositoryStub) GetLimitPoolFactsSnapshotForSystemAsOf(
	_ context.Context, _ string, _ string, _ string, _ time.Time,
) (app.LimitPoolFactsSnapshot, error) {
	return app.LimitPoolFactsSnapshot{}, snapshotport.ErrLimitPoolFactsSnapshotNotFound
}

func limitPoolApplicationPools() (*model.LimitPoolSnapshot, *model.LimitPoolSnapshot) {
	fetchedAt := time.Date(2026, 11, 27, 7, 1, 0, 0, time.UTC)
	up := &model.LimitPoolSnapshot{
		PoolType: "up", TradeDate: "2026-11-27", Total: 0, HasData: true,
		Source: "eastmoney", FetchedAt: fetchedAt, QualityStatus: model.QualityValid,
	}
	down := &model.LimitPoolSnapshot{
		PoolType: "down", TradeDate: "2026-11-27", Total: 0, HasData: true,
		Source: "eastmoney", FetchedAt: fetchedAt, QualityStatus: model.QualityValid,
	}
	return up, down
}

func TestLimitPoolFactsSnapshotCreatorPersistsAndAuthorizes(t *testing.T) {
	up, down := limitPoolApplicationPools()
	systemAsOf := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC) // 16:00 Asia/Shanghai
	owner, _, _ := newOrderBookTestOwner(t, systemAsOf, authorization.LimitPoolFactsPolicyRef)
	repository := &limitPoolSnapshotRepositoryStub{}
	creator, err := NewLimitPoolFactsSnapshotCreator(
		limitPoolReaderStub{up: up, down: down}, repository, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	refs, err := creator.Create(orderBookPrincipalContext(t), "600036.SH", "2026-11-27", systemAsOf, "risk_review")
	if err != nil {
		t.Fatalf("create limit-pool facts snapshot: %v", err)
	}
	if refs.SnapshotRef == "" || refs.AuthorizationRef == "" || !repository.hasSave {
		t.Fatalf("creator did not persist and authorize: %+v saved=%v", refs, repository.hasSave)
	}
	if repository.saved.Payload.LimitStatus != app.LimitPoolStatusNone ||
		repository.saved.Payload.TradeDate != "2026-11-27" {
		t.Fatalf("persisted payload = %+v", repository.saved.Payload)
	}
}

func TestLimitPoolFactsSnapshotCreatorRejectsPreCloseSystemAsOf(t *testing.T) {
	up, down := limitPoolApplicationPools()
	// 14:59 Asia/Shanghai == 06:59 UTC, before close.
	systemAsOf := time.Date(2026, 11, 27, 6, 59, 0, 0, time.UTC)
	owner, _, _ := newOrderBookTestOwner(t, systemAsOf, authorization.LimitPoolFactsPolicyRef)
	creator, err := NewLimitPoolFactsSnapshotCreator(
		limitPoolReaderStub{up: up, down: down}, &limitPoolSnapshotRepositoryStub{}, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creator.Create(orderBookPrincipalContext(t), "600036.SH", "2026-11-27", systemAsOf, "risk_review"); err == nil {
		t.Fatal("pre-close system_as_of must be rejected")
	}
}

func TestNewAuthorizedLimitPoolFactsSnapshotReaderRequiresRepository(t *testing.T) {
	if _, err := NewAuthorizedLimitPoolFactsSnapshotReader(nil); err == nil {
		t.Fatal("nil repository must be rejected")
	}
}
