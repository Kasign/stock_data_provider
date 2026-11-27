package application

import (
	"context"
	"errors"
	"strings"
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
	saved     app.LimitPoolFactsSnapshot
	hasSave   bool
	saveCount int
	// reuse 非空时模拟仓储返回一条可复用快照，用于验证 Creator 的 as-of 上界守卫。
	reuse *app.LimitPoolFactsSnapshot
}

func (s *limitPoolSnapshotRepositoryStub) SaveLimitPoolFactsSnapshot(_ context.Context, snapshot app.LimitPoolFactsSnapshot) error {
	s.saved, s.hasSave = snapshot, true
	s.saveCount++
	return nil
}

func (s *limitPoolSnapshotRepositoryStub) GetLimitPoolFactsSnapshot(_ context.Context, ref string) (app.LimitPoolFactsSnapshot, error) {
	if !s.hasSave || s.saved.Manifest.SnapshotRef != ref {
		return app.LimitPoolFactsSnapshot{}, snapshotport.ErrLimitPoolFactsSnapshotNotFound
	}
	return s.saved, nil
}

func (s *limitPoolSnapshotRepositoryStub) GetLatestLimitPoolFactsSnapshotAtOrBefore(
	_ context.Context, symbol, tradeDate, purpose string, asOf time.Time,
) (app.LimitPoolFactsSnapshot, error) {
	if s.reuse != nil {
		return *s.reuse, nil
	}
	if s.hasSave && s.saved.Manifest.Symbol == symbol && s.saved.Manifest.TradeDate == tradeDate &&
		s.saved.Manifest.PurposeCode == purpose {
		savedAsOf, err := time.Parse(time.RFC3339Nano, s.saved.Manifest.SystemAsOf)
		if err == nil && !savedAsOf.After(asOf) {
			return s.saved, nil
		}
	}
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

// TestLimitPoolFactsSnapshotCreatorRejectsReusePublishedAfterRequest 守住 as-of 上界：
// 即使仓储返回了一条「发布晚于评估时点」的快照（端口实现有缺陷时也可能发生），
// Creator 也必须拒绝，不能让未来事实进入评估。
func TestLimitPoolFactsSnapshotCreatorRejectsReusePublishedAfterRequest(t *testing.T) {
	up, down := limitPoolApplicationPools()
	systemAsOf := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC) // 16:00 Asia/Shanghai
	owner, _, _ := newOrderBookTestOwner(t, systemAsOf, authorization.LimitPoolFactsPolicyRef)
	reused := app.LimitPoolFactsSnapshot{Manifest: app.LimitPoolFactsManifest{
		SnapshotRef: "data_snapshot:sha256:" + repeatHexLocal(64, 'a'),
		Symbol:      "600036.SH", TradeDate: "2026-11-27", PurposeCode: "risk_review",
		SystemAsOf: systemAsOf.Add(time.Hour).Format(time.RFC3339Nano),
	}}
	creator, err := NewLimitPoolFactsSnapshotCreator(
		limitPoolReaderStub{up: up, down: down},
		&limitPoolSnapshotRepositoryStub{reuse: &reused}, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creator.Create(orderBookPrincipalContext(t), "600036.SH", "2026-11-27", systemAsOf, "risk_review"); err == nil {
		t.Fatal("a reused snapshot published after the requested system_as_of must be rejected")
	}
}

func repeatHexLocal(count int, value byte) string {
	return strings.Repeat(string(value), count)
}

// TestLimitPoolFactsSnapshotCollectorPersistsWithoutAuthorization 锁定采集器与授权签发
// 的分离：采集器没有 Owner 依赖，只固化事实，不产生授权决策。
func TestLimitPoolFactsSnapshotCollectorPersistsWithoutAuthorization(t *testing.T) {
	up, down := limitPoolApplicationPools()
	systemAsOf := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC) // 16:00 Asia/Shanghai
	repository := &limitPoolSnapshotRepositoryStub{}
	collector, err := NewLimitPoolFactsSnapshotCollector(limitPoolReaderStub{up: up, down: down}, repository)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := collector.Persist(orderBookPrincipalContext(t), "600036.SH", "2026-11-27", systemAsOf, "risk_review")
	if err != nil {
		t.Fatalf("collector persist: %v", err)
	}
	if ref == "" || !repository.hasSave || repository.saveCount != 1 {
		t.Fatalf("collector did not persist exactly one snapshot: ref=%q saved=%v saves=%d", ref, repository.hasSave, repository.saveCount)
	}
}

// TestLimitPoolFactsSnapshotCollectorIsIdempotentWithinAsOfWindow 确认重复采集不会重复造快照：
// 同一身份在更晚的评估时点复用的是已发布的那一条。
func TestLimitPoolFactsSnapshotCollectorIsIdempotentWithinAsOfWindow(t *testing.T) {
	up, down := limitPoolApplicationPools()
	systemAsOf := time.Date(2026, 11, 27, 8, 0, 0, 0, time.UTC)
	repository := &limitPoolSnapshotRepositoryStub{}
	collector, err := NewLimitPoolFactsSnapshotCollector(limitPoolReaderStub{up: up, down: down}, repository)
	if err != nil {
		t.Fatal(err)
	}
	first, err := collector.Persist(orderBookPrincipalContext(t), "600036.SH", "2026-11-27", systemAsOf, "risk_review")
	if err != nil {
		t.Fatal(err)
	}
	second, err := collector.Persist(orderBookPrincipalContext(t), "600036.SH", "2026-11-27", systemAsOf.Add(time.Hour), "risk_review")
	if err != nil {
		t.Fatal(err)
	}
	if first != second || repository.saveCount != 1 {
		t.Fatalf("collector must reuse the published snapshot: first=%s second=%s saves=%d", first, second, repository.saveCount)
	}
}

// TestLimitPoolFactsSnapshotCollectorStampsObservationInstant 锁定采集语义：
// 未指定时点时，快照时点必须等于实际观测时刻，而不是调用方事先取的「现在」。
func TestLimitPoolFactsSnapshotCollectorStampsObservationInstant(t *testing.T) {
	up, down := limitPoolApplicationPools()
	repository := &limitPoolSnapshotRepositoryStub{}
	collector, err := NewLimitPoolFactsSnapshotCollector(limitPoolReaderStub{up: up, down: down}, repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collector.Persist(orderBookPrincipalContext(t), "600036.SH", "2026-11-27", time.Time{}, "risk_review"); err != nil {
		t.Fatalf("persist with the observation instant: %v", err)
	}
	if !repository.hasSave {
		t.Fatal("collector did not persist")
	}
	if repository.saved.Manifest.SystemAsOf != repository.saved.Payload.ObservedAt.Format(time.RFC3339Nano) {
		t.Fatalf("manifest system_as_of=%q, want the observation instant %q",
			repository.saved.Manifest.SystemAsOf, repository.saved.Payload.ObservedAt.Format(time.RFC3339Nano))
	}
}
