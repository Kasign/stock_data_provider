package application_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	marketdataapp "example.com/stock-analysis-flow/internal/marketdata/app"
	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/marketdata/service"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"

	_ "github.com/go-sql-driver/mysql"
)

const (
	liveChainSymbol  = "600519.SH"
	liveChainPurpose = "risk_review"
)

// TestLiveLimitPoolFactsProductionChain 是 price_limit 的真实端到端证据：
// 真实 Eastmoney 全量涨跌停池 → Creator 摘要与授权签发 → MariaDB 不可变快照 →
// 按 limit-pool-facts-access 授权读回 → Quant 消费的 DTO。
//
// 它与 `TestLiveEastmoneyFullLimitPoolCompleteness` 互补：后者只验证 Provider 响应，
// 本用例验证「真实响应真的能走完生产写入与授权读取」。
//
// 时点说明（重要）：Creator 要求 observed_at 不晚于 system_as_of，所以生产语义是
// 「先采集、后在 system_as_of 评估」。本用例把评估时点取在采集之后，模拟这个顺序；
// 它不证明「Run 创建时刻早于采集」的在线顺序可用——那个顺序在契约上不成立，
// 见 doc/quant/17 第 9 节。
func TestLiveLimitPoolFactsProductionChain(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_LIMIT_POOL_CHAIN") != "1" {
		t.Skip("set MARKET_DATA_LIVE_LIMIT_POOL_CHAIN=1 to run the real limit-pool production chain")
	}
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB handle: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, repository.MigrationNamespace, repository.MigrationDefinitions()); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open Data store: %v", err)
	}
	defer store.Close()
	if err := store.ValidateSchema(ctx); err != nil {
		t.Fatalf("validate Data schema: %v", err)
	}

	now := time.Now().UTC()
	tradeDate := liveChainTradeDate(t, now)
	publishLiveChainLimitPoolPolicy(t, ctx, store, now)

	client := provider.NewEastmoney(20*time.Second, "", "")
	reader := service.NewWithCapabilities(
		[]provider.NamedProvider{client}, nil, nil, cache.NewDisabled(), store,
		time.Second, time.Second, 120, 2000, service.ResilienceConfig{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	reader.SetLimitPoolProviders([]provider.LimitPoolProvider{client})

	owner, err := marketdataapplication.NewDataAuthorizationOwner(store, store)
	if err != nil {
		t.Fatalf("create Data authorization owner: %v", err)
	}
	creator, err := marketdataapplication.NewLimitPoolFactsSnapshotCreator(reader, store, owner)
	if err != nil {
		t.Fatalf("create limit-pool facts Creator: %v", err)
	}
	authorizedReader, err := marketdataapplication.NewAuthorizedLimitPoolFactsSnapshotReader(store)
	if err != nil {
		t.Fatalf("create authorized limit-pool reader: %v", err)
	}

	principalCtx, err := platformsecurity.WithPrincipal(ctx, platformsecurity.Principal{
		Kind:       platformsecurity.IdentityService,
		AuthMethod: platformsecurity.AuthMethodServiceBearer,
		Subject:    "researchflow",
		Scopes:     []string{platformsecurity.ScopeDataRead},
	})
	if err != nil {
		t.Fatalf("attach trusted service principal: %v", err)
	}

	// 评估时点取在采集之后：这是 Creator 的合法顺序（先采集、后评估）。
	systemAsOf := now.Add(2 * time.Minute).Truncate(time.Second)
	refs, err := creator.Create(principalCtx, liveChainSymbol, tradeDate, systemAsOf, liveChainPurpose)
	if err != nil {
		t.Fatalf("create limit-pool facts snapshot from the real provider: %v", err)
	}
	if refs.SnapshotRef == "" || refs.AuthorizationRef == "" {
		t.Fatalf("creator returned empty references: %+v", refs)
	}

	snapshot, err := authorizedReader.Read(principalCtx, refs.SnapshotRef, refs.AuthorizationRef, liveChainPurpose)
	if err != nil {
		t.Fatalf("authorized read of the persisted snapshot: %v", err)
	}
	if err := snapshot.Verify(); err != nil {
		t.Fatalf("persisted snapshot failed verification: %v", err)
	}

	// 1) 快照身份与载荷版本必须是 Quant 消费的那个契约。
	if snapshot.Payload.SchemaVersion != marketdataapp.LimitPoolFactsPayloadSchemaVersion ||
		snapshot.Payload.PayloadKind != marketdataapp.LimitPoolFactsPayloadKind ||
		snapshot.Manifest.SchemaVersion == "" {
		t.Fatalf("payload kind/version mismatch: manifest=%q payload=%+v",
			snapshot.Manifest.SchemaVersion, snapshot.Payload)
	}
	if snapshot.Payload.Symbol != liveChainSymbol || snapshot.Payload.TradeDate != tradeDate {
		t.Fatalf("payload identity mismatch: symbol=%q trade_date=%q", snapshot.Payload.Symbol, snapshot.Payload.TradeDate)
	}

	// 2) 真实响应必须给出确定结论：日期可用（至少一个池有行），因此不允许 unknown。
	if snapshot.Payload.LimitStatus == marketdataapp.LimitPoolStatusUnknown {
		t.Fatalf("real provider produced an unknown limit status on %s: %+v", tradeDate, snapshot.Payload)
	}
	if !snapshot.Payload.UpPoolComplete || !snapshot.Payload.DownPoolComplete {
		t.Fatalf("real pools are not both complete: up=%v down=%v",
			snapshot.Payload.UpPoolComplete, snapshot.Payload.DownPoolComplete)
	}
	if snapshot.Payload.QualityStatus != marketdataapp.QualityValid &&
		snapshot.Payload.QualityStatus != marketdataapp.QualityDegraded {
		t.Fatalf("unexpected payload quality %q", snapshot.Payload.QualityStatus)
	}

	// 3) 精确复用：同一 (symbol, trade_date, purpose, system_as_of) 必须返回同一快照，
	//    不能因为重复调用再打一次 Provider 并生成新身份。
	reused, err := creator.Create(principalCtx, liveChainSymbol, tradeDate, systemAsOf, liveChainPurpose)
	if err != nil {
		t.Fatalf("exact reuse lookup failed: %v", err)
	}
	if reused.SnapshotRef != refs.SnapshotRef {
		t.Fatalf("reuse returned %s, want %s", reused.SnapshotRef, refs.SnapshotRef)
	}

	// 4) as-of 复用：更晚的评估时点（模拟 Run 在采集之后才创建）必须复用同一快照。
	//    这正是「先采集、后评估」的生产顺序，也是 A 方案要解决的问题。
	laterRun, err := creator.Create(principalCtx, liveChainSymbol, tradeDate, systemAsOf.Add(time.Hour), liveChainPurpose)
	if err != nil {
		t.Fatalf("as-of reuse for a later evaluation instant failed: %v", err)
	}
	if laterRun.SnapshotRef != refs.SnapshotRef {
		t.Fatalf("later evaluation instant got %s, want the earlier snapshot %s", laterRun.SnapshotRef, refs.SnapshotRef)
	}

	// 5) 跨数据类隔离：同一 ref 不能通过盘口读取入口拿到。
	if _, err := store.GetOrderBookFactsSnapshot(ctx, refs.SnapshotRef); err == nil {
		t.Fatal("limit-pool snapshot must not be readable as an order-book snapshot")
	}

	t.Logf("LIVE_LIMIT_POOL_CHAIN_OK trade_date=%s status=%s up=%d down=%d quality=%s snapshot=%s",
		tradeDate, snapshot.Payload.LimitStatus, snapshot.Payload.UpPoolTotal,
		snapshot.Payload.DownPoolTotal, snapshot.Payload.QualityStatus, snapshot.Manifest.SnapshotRef)
}

// TestLiveLimitPoolFactsRejectsEvaluationBeforeObservation 锁定 PIT 方向：
// 评估时点早于采集时会话必须被拒绝，否则「用未来数据回答过去的问题」。
func TestLiveLimitPoolFactsRejectsEvaluationBeforeObservation(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_LIMIT_POOL_CHAIN") != "1" {
		t.Skip("set MARKET_DATA_LIVE_LIMIT_POOL_CHAIN=1 to run the real limit-pool production chain")
	}
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB handle: %v", err)
	}
	defer db.Close()
	if err := platformstorage.ApplyMigrations(ctx, db, repository.MigrationNamespace, repository.MigrationDefinitions()); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open Data store: %v", err)
	}
	defer store.Close()

	now := time.Now().UTC()
	tradeDate := liveChainTradeDate(t, now)
	publishLiveChainLimitPoolPolicy(t, ctx, store, now)

	client := provider.NewEastmoney(20*time.Second, "", "")
	reader := service.NewWithCapabilities(
		[]provider.NamedProvider{client}, nil, nil, cache.NewDisabled(), store,
		time.Second, time.Second, 120, 2000, service.ResilienceConfig{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	reader.SetLimitPoolProviders([]provider.LimitPoolProvider{client})
	owner, err := marketdataapplication.NewDataAuthorizationOwner(store, store)
	if err != nil {
		t.Fatalf("create Data authorization owner: %v", err)
	}
	creator, err := marketdataapplication.NewLimitPoolFactsSnapshotCreator(reader, store, owner)
	if err != nil {
		t.Fatalf("create limit-pool facts Creator: %v", err)
	}

	principalCtx, err := platformsecurity.WithPrincipal(ctx, platformsecurity.Principal{
		Kind:       platformsecurity.IdentityService,
		AuthMethod: platformsecurity.AuthMethodServiceBearer,
		Subject:    "researchflow",
		Scopes:     []string{platformsecurity.ScopeDataRead},
	})
	if err != nil {
		t.Fatalf("attach trusted service principal: %v", err)
	}

	// 评估时点比采集早一小时：真实响应不可能满足 observed_at <= system_as_of。
	early := now.Add(-time.Hour)
	if _, err := creator.Create(principalCtx, liveChainSymbol, tradeDate, early, liveChainPurpose); err == nil {
		t.Fatal("evaluation instant before the observation must be rejected")
	} else if !errors.Is(err, marketdataapplication.ErrLimitPoolFactsSnapshotStaleAsOf) {
		t.Fatalf("rejection = %v, want ErrLimitPoolFactsSnapshotStaleAsOf", err)
	}
}

// liveChainTradeDate 返回最近一个已收盘的交易日（不含周末）。收盘前当日池不完整，
// 必须退到上一个工作日，否则 Creator 的收盘约束会拒绝。
func liveChainTradeDate(t *testing.T, now time.Time) string {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load Asia/Shanghai: %v", err)
	}
	local := now.In(loc)
	latest := local
	if local.Hour() < 15 || (local.Hour() == 15 && local.Minute() < 30) {
		latest = local.AddDate(0, 0, -1)
	}
	for offset := 0; offset < 10; offset++ {
		day := latest.AddDate(0, 0, -offset)
		if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
			continue
		}
		return day.Format("2006-01-02")
	}
	t.Fatal("no weekday trade date within 10 days")
	return ""
}

// publishLiveChainLimitPoolPolicy 发布 limit-pool-facts-access 的当前策略。
// 它是测试专用策略（主体 service:researchflow、用途 risk_review），不改变生产策略；
// 已存在兼容策略时直接复用，避免同一数据库上第二次运行因版本冲突失败。
func publishLiveChainLimitPoolPolicy(t *testing.T, ctx context.Context, store *repository.Store, now time.Time) {
	t.Helper()
	expectedCurrent := ""
	current, err := store.ResolveCurrentDataAuthorizationPolicyByRef(ctx, authorization.LimitPoolFactsPolicyRef)
	switch {
	case err == nil:
		if liveChainPolicyCovers(current.Policy) {
			return
		}
		expectedCurrent = current.Version.PolicyVersion
	case errors.Is(err, marketdataapp.ErrDataAuthorizationPolicyNotPublished):
	default:
		t.Fatalf("resolve current limit-pool access policy: %v", err)
	}

	policy := authorization.DataAccessPolicy{
		SchemaVersion:   authorization.DataAccessPolicySchemaVersion,
		Status:          authorization.PolicyApproved,
		AllowedSubjects: []string{"principal:service:researchflow"},
		AllowedPurposes: []string{liveChainPurpose},
		RequiredScopes:  []string{platformsecurity.ScopeDataRead},
		EffectiveAt:     now.Add(-time.Hour).UTC(),
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatalf("marshal data access policy: %v", err)
	}
	sum := sha256.Sum256(payload)
	// 版本必须每次不同：同一版本号换载荷会被仓储判为冲突。
	version := "live-chain-" + now.Format("20060102T150405.000000000")
	value := authorization.PolicyVersion{
		PolicyType:    authorization.DataAccessPolicyType,
		PolicyRef:     authorization.LimitPoolFactsPolicyRef,
		PolicyVersion: version,
		Payload:       payload,
		PayloadHash:   hex.EncodeToString(sum[:]),
		CreatedAt:     now,
	}
	audit := marketdataapp.DataAuthorizationPolicyPublicationAudit{
		ActorRef:   "principal:owner",
		RequestID:  "live-limit-pool-chain-" + now.Format("20060102150405"),
		OccurredAt: now,
	}
	if err := store.PublishDataAuthorizationPolicyVersion(ctx, expectedCurrent, value, audit); err != nil {
		t.Fatalf("publish limit-pool access policy: %v", err)
	}
}

// liveChainPolicyCovers 判断已发布的策略是否已满足本用例的访问需求。
func liveChainPolicyCovers(policy authorization.DataAccessPolicy) bool {
	if policy.Status != authorization.PolicyApproved {
		return false
	}
	hasSubject, hasPurpose, hasScope := false, false, false
	for _, subject := range policy.AllowedSubjects {
		if subject == "principal:service:researchflow" {
			hasSubject = true
		}
	}
	for _, purpose := range policy.AllowedPurposes {
		if purpose == liveChainPurpose {
			hasPurpose = true
		}
	}
	for _, scope := range policy.RequiredScopes {
		if scope == platformsecurity.ScopeDataRead {
			hasScope = true
		}
	}
	return hasSubject && hasPurpose && hasScope
}
