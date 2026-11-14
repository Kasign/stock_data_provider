package bootstrap

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	platformauth "example.com/stock-analysis-flow/internal/platform/authorization"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	researchnews "example.com/stock-analysis-flow/internal/research/adapters/news"
	researchstorage "example.com/stock-analysis-flow/internal/research/storage"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
)

// 本文件是 News→Research 跨模块 MariaDB 联调验收（交接文档第 3 节第 3 项的 A 组）。
// 与 research_news_test.go 的空壳 &sql.DB{} 构造测试不同，这里所有依赖都是真实实例：
// 真实 MariaDB、真实双 namespace Migration、真实 Server 组合根、真实冻结 Golden。
// B 组（Research Evidence 成功持久化）因 News 不提供 available_at 而被 blocked，
// 见本文件 TestNewsResearchMariaDBIntegration/authorized_read_and_fail_closed_import。

const (
	newsResearchDSNEnv    = "SERVER_RELAY_TEST_MYSQL_DSN"
	newsResearchNewsID    = "news_001"
	newsResearchCallerRef = "service:researchflow"
	newsResearchActiveKey = "active-v2"
	newsResearchOldKey    = "previous-v1"
)

var newsResearchDatabaseSeq atomic.Uint64

// openNewsResearchMariaDB 复用 Integration CI 已有的 admin DSN，按 relay 集成测试的
// 范式创建独立 database，避免与共用 integration_relay_test 的模块级测试互相污染。
func openNewsResearchMariaDB(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	dsn := os.Getenv(newsResearchDSNEnv)
	if dsn == "" {
		t.Skipf("%s 未配置，跳过 News→Research MariaDB 集成测试", newsResearchDSNEnv)
	}
	admin, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("打开 MariaDB admin 连接失败: %v", err)
	}
	if err := admin.PingContext(ctx); err != nil {
		_ = admin.Close()
		t.Fatalf("连接 MariaDB 失败: %v", err)
	}
	database := fmt.Sprintf("news_research_%d_%d", time.Now().UnixNano(), newsResearchDatabaseSeq.Add(1))
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+database+"`"); err != nil {
		_ = admin.Close()
		t.Fatalf("创建隔离 database %q 失败: %v", database, err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), "DROP DATABASE IF EXISTS `"+database+"`")
		_ = admin.Close()
	})

	parsed, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", newsResearchDSNEnv, err)
	}
	parsed.DBName = database
	connector, err := mysql.NewConnector(parsed)
	if err != nil {
		t.Fatalf("创建隔离 MariaDB connector 失败: %v", err)
	}
	db := sql.OpenDB(connector)
	db.SetMaxOpenConns(8)
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("连接隔离 database 失败: %v", err)
	}

	// 与生产 Bootstrap 一致：两个 namespace 在同一 DB 上共存，表名前缀互不冲突。
	registry := servermigration.NewRegistry()
	if err := registry.Register("news", newsstorage.Migrations()); err != nil {
		t.Fatalf("注册 News Migration 失败: %v", err)
	}
	if err := registry.Register(researchstorage.MigrationNamespace, researchstorage.Migrations()); err != nil {
		t.Fatalf("注册 Research Migration 失败: %v", err)
	}
	if _, err := registry.RunWithReport(ctx, db); err != nil {
		t.Fatalf("应用 News/Research Migration 失败: %v", err)
	}
	return db
}

func loadNewsResearchCursorConfig(t *testing.T) newsconfig.Config {
	t.Helper()
	values := map[string]string{
		newsconfig.EvidenceCursorActiveKeyIDEnv: newsResearchActiveKey,
		newsconfig.EvidenceCursorKeyRingEnv:     `{"active-v2":"0123456789abcdef0123456789abcdef","previous-v1":"abcdef0123456789abcdef0123456789"}`,
	}
	cfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}))
	if err != nil {
		t.Fatalf("加载 News Cursor Key Ring 配置失败: %v", err)
	}
	return cfg
}

// loadNewsResearchGoldenBinding 读取项目级冻结 Golden，绝不手写授权对象。
func loadNewsResearchGoldenBinding(t *testing.T) newsdomain.NewsAuthorizationBinding {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "整体任务规划", "golden", "NEWS-CTR-002-authorization-binding.golden.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取 NEWS-CTR-002 Golden 失败: %v", err)
	}
	var fixture struct {
		AuthorizationBinding newsdomain.NewsAuthorizationBinding `json:"authorization_binding"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatalf("解析 NEWS-CTR-002 Golden 失败: %v", err)
	}
	if err := newsdomain.ValidateNewsAuthorizationBinding(fixture.AuthorizationBinding); err != nil {
		t.Fatalf("NEWS-CTR-002 Golden 不合法: %v", err)
	}
	return fixture.AuthorizationBinding
}

// makeAllowedBinding 把 Golden 的 conditional 决策改成 allowed，并用公共
// GOV-CTR-006 JCS 实现重算指纹。绝不硬编码或跳过指纹校验。
func makeAllowedBinding(t *testing.T, golden newsdomain.NewsAuthorizationBinding) newsdomain.NewsAuthorizationBinding {
	t.Helper()
	binding := golden
	binding.AuthorizationRef.Decision = newsdomain.AuthorizationDecisionAllowed
	binding.AuthorizationRef.Conditions = []string{} // 必须是非 nil 空切片
	expiry := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	binding.AuthorizationRef.ExpiresAt = newsdomain.NullableDateTime{Present: true, Value: &expiry}
	refJSON, err := json.Marshal(binding.AuthorizationRef)
	if err != nil {
		t.Fatalf("序列化 AuthorizationRef 失败: %v", err)
	}
	snapshotJSON, err := json.Marshal(binding.PolicySnapshot)
	if err != nil {
		t.Fatalf("序列化 PolicySnapshot 失败: %v", err)
	}
	fingerprint, err := platformauth.Fingerprint(refJSON, snapshotJSON)
	if err != nil {
		t.Fatalf("重算 GOV-CTR-006 决策指纹失败: %v", err)
	}
	binding.AuthorizationRef.DecisionFingerprint = fingerprint
	return binding
}

// newsResearchSeedObservation 构造与 NEWS-CTR-002 Golden 策略轨迹完全匹配的
// Observation。单独抽出以便无 DB 的自检测试也能验证这段最易出错的字段映射。
func newsResearchSeedObservation(observedAt, retentionUntil time.Time) newsdomain.Observation {
	candidate := newsdomain.NormalizeCandidate(newsdomain.Candidate{
		SourceItemID: "sec-001",
		Title:        "SEC filing for News/Research integration",
	}, observedAt, "")
	return newsdomain.Observation{
		ID: "news-it-auth-observation", IdempotencyKey: "news-it-auth-observation-key",
		NewsID: newsResearchNewsID, SourceID: "sec-edgar", ProviderCode: "tavily",
		SourcePolicyVersionID: "2.1.0",
		EndpointID:            "sec-edgar-filings", EndpointPolicyVersionID: "1.0.0",
		ProviderOutputPolicyRef: "tavily-output", ProviderOutputPolicyVersionID: "3.0.0",
		IdentityStatus: newsdomain.ObservationNew,
		ObservedAt:     observedAt, RetentionUntil: retentionUntil,
		Candidate: candidate,
	}
}

// newsResearchPolicyPayloads 返回与 Golden PolicySnapshot 三类策略对应的当前策略载荷。
func newsResearchPolicyPayloads() map[string]any {
	purposes := []newsdomain.Purpose{newsdomain.PurposeInternalPersonalResearch}
	kinds := []newsdomain.ContentKind{newsdomain.ContentSnippet}
	return map[string]any{
		newsdomain.NewsPolicyTypeSource: newsdomain.SourcePolicy{
			Status: newsdomain.AuthorizationApproved, AllowedPurposes: purposes,
			AllowedContentKinds: kinds, EvidenceRef: "integration-source-evidence",
		},
		newsdomain.NewsPolicyTypeEndpoint: newsdomain.EndpointPolicy{},
		newsdomain.NewsPolicyTypeProviderOutput: newsdomain.ProviderOutputPolicy{
			Status: newsdomain.AuthorizationApproved, AllowedPurposes: purposes,
			AllowedContentKinds: kinds, AllowRequestProcessing: true,
			EvidenceRef: "integration-provider-evidence",
		},
	}
}

type newsResearchFixture struct {
	db          *sql.DB
	repo        *newsstorage.MySQLRepository
	composition *researchNewsComposition
	binding     newsdomain.NewsAuthorizationBinding
	observation newsdomain.Observation
	version     newsdomain.NewsItemVersion
	systemAsOf  time.Time
}

func (f *newsResearchFixture) getRequest() newsapp.GetEvidenceRequest {
	return newsapp.GetEvidenceRequest{
		CallerRef:             newsResearchCallerRef,
		PurposeCode:           newsapp.EvidencePurposeResearchAnalysis,
		RequestedContentLevel: newsapp.EvidenceContentSnippet,
		SystemAsOf:            f.systemAsOf,
		NewsID:                newsResearchNewsID,
		VersionID:             f.version.ID,
	}
}

func (f *newsResearchFixture) listRequest() newsapp.ListEvidenceRequest {
	return newsapp.ListEvidenceRequest{
		CallerRef:             newsResearchCallerRef,
		PurposeCode:           newsapp.EvidencePurposeResearchAnalysis,
		RequestedContentLevel: newsapp.EvidenceContentSnippet,
		SystemAsOf:            f.systemAsOf,
		Filters:               &newsapp.EvidenceQueryFilters{},
		PageSize:              10,
	}
}

// newNewsResearchFixture 用 News 公开 Application / Repository 入口在真实 MariaDB
// 中播种一条可被 EvidenceQueryFacade 成功读取的 Evidence。播种顺序与字段约束来自
// NEWS-CTR-002 Golden、ValidateNewsAuthorizationBindingForEvidence 和
// validEvidencePITRecord；不修改任何模块领域代码，也不使用 Mock Facade。
func newNewsResearchFixture(t *testing.T, ctx context.Context) *newsResearchFixture {
	t.Helper()
	db := openNewsResearchMariaDB(t, ctx)
	repo, err := newsstorage.NewMySQLRepository(db)
	if err != nil {
		t.Fatalf("构造 News MySQL Repository 失败: %v", err)
	}
	composition, err := newResearchNewsComposition(db, loadNewsResearchCursorConfig(t))
	if err != nil {
		t.Fatalf("在真实 MariaDB 上构造 News→Research 组合根失败: %v", err)
	}
	binding := makeAllowedBinding(t, loadNewsResearchGoldenBinding(t))

	// 公开 Ingest 入口的 NewsID 由存储随机生成，无法对齐冻结 Golden 的
	// resource_ref=news_evidence_version:news_001:v1。测试自建库中补一行
	// news_items 以满足外键与 LatestNewsItemVersion 的 FOR UPDATE 语义。
	if _, err := db.ExecContext(ctx,
		"INSERT INTO news_items (news_id, origin_source_id) VALUES (?, ?)",
		newsResearchNewsID, "sec-edgar",
	); err != nil {
		t.Fatalf("播种 news_items 失败: %v", err)
	}

	observedAt := time.Now().UTC().Add(-time.Hour)
	retentionUntil := time.Now().UTC().Add(24 * time.Hour)
	observation := newsResearchSeedObservation(observedAt, retentionUntil)
	candidate := observation.Candidate
	if err := repo.WithinIdentityTransaction(ctx, func(tx newsapp.IdentityTransaction) error {
		_, appendErr := tx.AppendObservation(ctx, observation)
		return appendErr
	}); err != nil {
		t.Fatalf("播种 Observation 失败（检查 news_items 外键与 NOT NULL 列）: %v", err)
	}

	updater := newsapp.NewApplyResearchUpdate(repo, func() string { return "news-it-auth-version" })
	result, err := updater.Execute(ctx, newsapp.ResearchUpdateRequest{
		NewsID: newsResearchNewsID, ObservationID: observation.ID,
		Candidate: candidate,
		Content: newsdomain.ContentLayers{
			SourceSummary:       "SEC filing source summary",
			SourceSummaryOrigin: newsdomain.ContentOriginFeedSummary,
		},
	})
	if err != nil {
		t.Fatalf("播种 NewsItemVersion 失败: %v", err)
	}
	if !result.VersionCreated || result.Version.VersionNo != 1 {
		t.Fatalf("播种版本结果异常: created=%v version_no=%d", result.VersionCreated, result.Version.VersionNo)
	}

	// 当前策略指针必须与 Golden PolicySnapshot 的 (type, ref, version) 完全一致。
	publisher, err := newsapp.NewPublishNewsPolicyVersion(repo)
	if err != nil {
		t.Fatalf("构造策略发布器失败: %v", err)
	}
	payloads := newsResearchPolicyPayloads()
	for _, policy := range binding.PolicySnapshot.Policies {
		payload, err := json.Marshal(payloads[policy.PolicyType])
		if err != nil {
			t.Fatalf("序列化 %s 策略失败: %v", policy.PolicyType, err)
		}
		if err := publisher.Execute(ctx, policy.PolicyType, policy.PolicyRef, policy.PolicyVersion, payload); err != nil {
			t.Fatalf("发布 %s/%s/%s 策略失败: %v", policy.PolicyType, policy.PolicyRef, policy.PolicyVersion, err)
		}
	}

	// SystemAsOf 必须在版本观察时间之后、且不晚于 GetEvidence 内部 now。
	systemAsOf := time.Now().UTC()
	pit, err := repo.GetEvidenceVersionAt(ctx, newsapp.GetEvidenceRequest{
		CallerRef: newsResearchCallerRef, PurposeCode: newsapp.EvidencePurposeResearchAnalysis,
		RequestedContentLevel: newsapp.EvidenceContentSnippet,
		SystemAsOf:            systemAsOf, NewsID: newsResearchNewsID, VersionID: result.Version.ID,
	})
	if err != nil {
		t.Fatalf("读回 PIT 记录失败: %v", err)
	}
	if err := repo.AppendAuthorizationSnapshot(ctx, binding, pit.Observation, pit.Version); err != nil {
		t.Fatalf("写入 NEWS-CTR-002 授权快照失败（检查 Observation 策略轨迹字段）: %v", err)
	}

	return &newsResearchFixture{
		db: db, repo: repo, composition: composition, binding: binding,
		observation: observation, version: result.Version, systemAsOf: systemAsOf,
	}
}

// TestNewsResearchIntegrationFixturesAreValid 在没有 MariaDB 的环境下真实验证
// 最容易导致 CI 首次运行失败的纯逻辑前置条件：Golden 决策改写、GOV-CTR-006 JCS
// 指纹重算、三类策略载荷合法性，以及播种 Observation 与冻结契约的字段匹配。
// 这些断言不依赖数据库，但覆盖了跨模块播种的主要约束校验点。
func TestNewsResearchIntegrationFixturesAreValid(t *testing.T) {
	binding := makeAllowedBinding(t, loadNewsResearchGoldenBinding(t))
	if err := newsdomain.ValidateNewsAuthorizationBinding(binding); err != nil {
		t.Fatalf("改写后的授权绑定不合法: %v", err)
	}
	refJSON, err := json.Marshal(binding.AuthorizationRef)
	if err != nil {
		t.Fatalf("序列化 AuthorizationRef 失败: %v", err)
	}
	snapshotJSON, err := json.Marshal(binding.PolicySnapshot)
	if err != nil {
		t.Fatalf("序列化 PolicySnapshot 失败: %v", err)
	}
	if err := platformauth.VerifyFingerprint(refJSON, snapshotJSON); err != nil {
		t.Fatalf("重算后的 GOV-CTR-006 指纹未通过公共 Verifier: %v", err)
	}
	if use := newsdomain.EvaluateNewsAuthorizationUse(binding, time.Now().UTC()); !use.Allowed {
		t.Fatalf("改写后的授权快照仍不可用: %+v", use)
	}

	observation := newsResearchSeedObservation(
		time.Now().UTC().Add(-time.Hour), time.Now().UTC().Add(24*time.Hour))
	version := newsdomain.NewsItemVersion{
		ID: "news-it-auth-version", NewsID: newsResearchNewsID, VersionNo: 1,
		ObservationID: observation.ID,
	}
	if err := newsdomain.ValidateNewsAuthorizationBindingForEvidence(binding, observation, version); err != nil {
		t.Fatalf("播种 Observation/Version 与冻结授权绑定不匹配: %v", err)
	}

	payloads := newsResearchPolicyPayloads()
	for _, policy := range binding.PolicySnapshot.Policies {
		payload, err := json.Marshal(payloads[policy.PolicyType])
		if err != nil {
			t.Fatalf("序列化 %s 策略失败: %v", policy.PolicyType, err)
		}
		sum := sha256.Sum256(payload)
		value := newsdomain.NewsPolicyVersion{
			PolicyType: policy.PolicyType, PolicyRef: policy.PolicyRef,
			PolicyVersion: policy.PolicyVersion, Payload: payload,
			PayloadHash: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UTC(),
		}
		if err := newsdomain.ValidateNewsPolicyVersion(value); err != nil {
			t.Fatalf("%s 当前策略载荷不合法: %v", policy.PolicyType, err)
		}
	}
}

// TestNewsEvidenceCursorKeyRotation 不依赖 MariaDB，用真实 EvidenceCursorCodec
// 验证 15 分钟 TTL、旧 Key 轮换期间验签、签名篡改与旧 Key 提前移除的拒绝行为。
func TestNewsEvidenceCursorKeyRotation(t *testing.T) {
	cfg := loadNewsResearchCursorConfig(t)
	activeKeyID, keyRing, configured := cfg.EvidenceCursorKeyRing()
	if !configured || activeKeyID != newsResearchActiveKey {
		t.Fatalf("Key Ring 配置异常: configured=%v active=%q", configured, activeKeyID)
	}
	previousCodec, err := newsapp.NewEvidenceCursorCodec(newsResearchOldKey, keyRing)
	if err != nil {
		t.Fatalf("用旧 Key 构造 Codec 失败: %v", err)
	}
	activeCodec, err := newsapp.NewEvidenceCursorCodec(newsResearchActiveKey, keyRing)
	if err != nil {
		t.Fatalf("用当前 Key 构造 Codec 失败: %v", err)
	}

	issuedAt := time.Now().UTC()
	request := newsapp.ListEvidenceRequest{
		CallerRef: newsResearchCallerRef, PurposeCode: newsapp.EvidencePurposeResearchAnalysis,
		RequestedContentLevel: newsapp.EvidenceContentSnippet,
		SystemAsOf:            issuedAt.Add(-time.Minute),
		Filters:               &newsapp.EvidenceQueryFilters{},
		PageSize:              10,
	}
	last := newsapp.ResearchEvidenceRecord{
		NewsID: newsResearchNewsID, VersionID: "news-it-auth-version",
		SystemObservedAt: request.SystemAsOf, SystemAsOf: request.SystemAsOf,
	}
	token, err := previousCodec.EncodeNextCursor(request, "snapshot_ref_001", last, issuedAt)
	if err != nil {
		t.Fatalf("用轮换前 Key 签发续页游标失败: %v", err)
	}
	now := issuedAt.Add(time.Second)

	// 轮换后 active Key 已切换，但 Key Ring 保留旧 Key，旧游标仍应可验签。
	position, err := activeCodec.DecodeCursor(token, request, now)
	if err != nil {
		t.Fatalf("旧 Key 签发的游标在轮换期间无法验签: %v", err)
	}
	if position.SnapshotRef != "snapshot_ref_001" || position.LastNewsID != newsResearchNewsID {
		t.Fatalf("解码出的续页位置异常: %+v", position)
	}
	if !position.ExpiresAt.Equal(issuedAt.Add(15 * time.Minute)) {
		t.Fatalf("游标 TTL = %v, want 15 分钟", position.ExpiresAt.Sub(issuedAt))
	}

	// 篡改签名必须被拒绝。
	parts := strings.Split(token, ".")
	if len(parts) != 2 || len(parts[1]) == 0 {
		t.Fatalf("游标格式异常: %q", token)
	}
	signature := []byte(parts[1])
	if signature[0] == 'A' {
		signature[0] = 'B'
	} else {
		signature[0] = 'A'
	}
	tampered := parts[0] + "." + string(signature)
	if _, err := activeCodec.DecodeCursor(tampered, request, now); !errors.Is(err, newsapp.ErrEvidenceCursorInvalid) {
		t.Fatalf("篡改签名的游标错误 = %v, want ErrEvidenceCursorInvalid", err)
	}

	// 超过 15 分钟 TTL 必须过期，不得继续用于续页。
	if _, err := activeCodec.DecodeCursor(token, request, issuedAt.Add(16*time.Minute)); !errors.Is(err, newsapp.ErrEvidenceCursorExpired) {
		t.Fatalf("过期游标错误 = %v, want ErrEvidenceCursorExpired", err)
	}

	// 轮换窗口内若提前移除旧 Key，旧游标必须 fail-closed 拒绝。
	withoutOldKey, err := newsapp.NewEvidenceCursorCodec(newsResearchActiveKey,
		map[string]platformconfig.Secret{newsResearchActiveKey: keyRing[newsResearchActiveKey]})
	if err != nil {
		t.Fatalf("构造仅含当前 Key 的 Codec 失败: %v", err)
	}
	if _, err := withoutOldKey.DecodeCursor(token, request, now); !errors.Is(err, newsapp.ErrEvidenceCursorInvalid) {
		t.Fatalf("旧 Key 提前移除后的错误 = %v, want ErrEvidenceCursorInvalid", err)
	}
}

// TestNewsResearchMariaDBIntegration 是 A 组跨模块端到端验收。
// 破坏性用例（追加篡改快照、切换策略指针）各自使用独立 fixture，避免相互污染。
func TestNewsResearchMariaDBIntegration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()

	t.Run("read_chain_and_fail_closed_import", func(t *testing.T) {
		f := newNewsResearchFixture(t, ctx)

		t.Run("composition_and_schema", func(t *testing.T) {
			if f.composition == nil || f.composition.query == nil || f.composition.research == nil ||
				f.composition.importer == nil || f.composition.authorizer == nil {
				t.Fatal("真实组合根未构造出全部生产依赖")
			}
			missing, err := f.repo.MissingSchemaTables(ctx)
			if err != nil || len(missing) != 0 {
				t.Fatalf("News Schema 自检失败: missing=%v err=%v", missing, err)
			}
			var tables int
			if err := f.db.QueryRowContext(ctx,
				"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('news_item_versions', 'research_evidence_records')",
			).Scan(&tables); err != nil {
				t.Fatalf("检查双 namespace 表失败: %v", err)
			}
			if tables != 2 {
				t.Fatalf("双 namespace 迁移后目标表数量 = %d, want 2", tables)
			}
		})

		t.Run("authorized_read_and_fail_closed_import", func(t *testing.T) {
			record, err := f.composition.query.GetEvidence(ctx, f.getRequest())
			if err != nil {
				t.Fatalf("真实 MariaDB 授权读取失败（依次检查策略指针、Golden 指纹、PIT 时间）: %v", err)
			}
			if record.NewsID != newsResearchNewsID || record.VersionNo != 1 ||
				record.AuthorizationStatus != "allowed" || record.AuthorizationRef == "" {
				t.Fatalf("授权读取结果异常: %+v", record)
			}
			// 当前契约事实：News 不提供可验证的 available_at，返回 null。
			// 这是 B 组成功写入链路被 blocked 的直接原因，显式断言防止被误改。
			if record.AvailableAt != nil {
				t.Fatalf("News 返回了未预期的 available_at: %v", *record.AvailableAt)
			}

			_, err = f.composition.importer.ImportEvidence(ctx, researchnews.ImportRequest{
				CallerRef: newsResearchCallerRef, PurposeCode: "research_analysis",
				RequestedContentLevel: "snippet", SystemAsOf: f.systemAsOf,
				NewsID: newsResearchNewsID, VersionID: f.version.ID,
			})
			if !errors.Is(err, researchnews.ErrEvidenceNotPITSafe) {
				t.Fatalf("Importer 错误 = %v, want ErrEvidenceNotPITSafe（fail-closed）", err)
			}
			var persisted int
			if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM research_evidence_records").Scan(&persisted); err != nil {
				t.Fatalf("读取 Research Evidence 表失败: %v", err)
			}
			if persisted != 0 {
				t.Fatalf("fail-closed 后 Research Evidence 落库行数 = %d, want 0", persisted)
			}
		})

		t.Run("pit_invisible", func(t *testing.T) {
			request := f.getRequest()
			request.SystemAsOf = f.observation.ObservedAt.Add(-time.Second)
			record, err := f.composition.query.GetEvidence(ctx, request)
			if err == nil {
				t.Fatal("SystemAsOf 早于版本观察时间时仍返回了 Evidence")
			}
			if record.NewsID != "" || record.ContentExcerpt != nil {
				t.Fatalf("PIT 不可见时泄露了内容 DTO: %+v", record)
			}
			t.Logf("PIT 不可见返回错误: %v", err)
		})

		t.Run("context_cancelled", func(t *testing.T) {
			canceled, cancelCall := context.WithCancel(ctx)
			cancelCall()
			record, err := f.composition.query.GetEvidence(canceled, f.getRequest())
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("取消上下文后的错误 = %v, want context.Canceled", err)
			}
			if record.NewsID != "" {
				t.Fatalf("取消上下文后仍返回了内容 DTO: %+v", record)
			}
		})

		t.Run("list_evidence_page_uses_current_authorization", func(t *testing.T) {
			page, err := f.composition.query.ListEvidence(ctx, f.listRequest())
			if err != nil {
				t.Fatalf("真实快照分页失败: %v", err)
			}
			if page.SnapshotRef == "" || !page.SystemAsOf.Equal(f.systemAsOf) {
				t.Fatalf("快照页边界异常: %+v", page)
			}
			if len(page.Items) != 1 || page.Items[0].NewsID != newsResearchNewsID {
				t.Fatalf("快照页条目异常: %+v", page.Items)
			}
			if page.Items[0].AuthorizationStatus != "allowed" || page.Items[0].AvailableAt != nil {
				t.Fatalf("分页条目未逐条执行当前授权复核: %+v", page.Items[0])
			}
		})
	})

	t.Run("fingerprint_tamper_rejected", func(t *testing.T) {
		f := newNewsResearchFixture(t, ctx)
		tampered := f.binding
		tampered.AuthorizationRef.AuthorizationRef += "-tampered"
		tampered.AuthorizationRef.DecidedAt = time.Now().UTC().Format(time.RFC3339Nano)
		expiry := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
		tampered.AuthorizationRef.ExpiresAt = newsdomain.NullableDateTime{Present: true, Value: &expiry}
		tampered.AuthorizationRef.DecisionFingerprint = strings.Repeat("0", 64)
		if err := f.repo.AppendAuthorizationSnapshot(ctx, tampered, f.observation, f.version); err != nil {
			t.Fatalf("追加篡改指纹的授权快照失败: %v", err)
		}
		record, err := f.composition.query.GetEvidence(ctx, f.getRequest())
		if !errors.Is(err, newsapp.ErrNewsAuthorizationFingerprintInvalid) {
			t.Fatalf("篡改指纹后的错误 = %v, want ErrNewsAuthorizationFingerprintInvalid", err)
		}
		if record.NewsID != "" || record.ContentExcerpt != nil {
			t.Fatalf("指纹校验失败时泄露了内容 DTO: %+v", record)
		}
	})

	t.Run("current_policy_version_change_rejected", func(t *testing.T) {
		f := newNewsResearchFixture(t, ctx)
		publisher, err := newsapp.NewPublishNewsPolicyVersion(f.repo)
		if err != nil {
			t.Fatalf("构造策略发布器失败: %v", err)
		}
		payload, err := json.Marshal(newsdomain.SourcePolicy{
			Status:              newsdomain.AuthorizationApproved,
			AllowedPurposes:     []newsdomain.Purpose{newsdomain.PurposeInternalPersonalResearch},
			AllowedContentKinds: []newsdomain.ContentKind{newsdomain.ContentSnippet},
			EvidenceRef:         "integration-source-evidence",
		})
		if err != nil {
			t.Fatalf("序列化撤销用策略失败: %v", err)
		}
		if err := publisher.Execute(ctx, newsdomain.NewsPolicyTypeSource, "sec-edgar", "2.2.0", payload); err != nil {
			t.Fatalf("切换当前 source 策略指针失败: %v", err)
		}
		record, err := f.composition.query.GetEvidence(ctx, f.getRequest())
		if err == nil {
			t.Fatal("当前策略版本已变更（授权快照不再匹配）时仍返回了 Evidence")
		}
		if record.NewsID != "" || record.ContentExcerpt != nil {
			t.Fatalf("策略版本变更时泄露了内容 DTO: %+v", record)
		}
		t.Logf("当前策略版本变更返回错误: %v", err)
	})
}
