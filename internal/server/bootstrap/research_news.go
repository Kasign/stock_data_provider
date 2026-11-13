package bootstrap

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	newsplatform "example.com/stock-analysis-flow/internal/news/adapters/platform"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	researchnews "example.com/stock-analysis-flow/internal/research/adapters/news"
	researchapplication "example.com/stock-analysis-flow/internal/research/application"
	researchstorage "example.com/stock-analysis-flow/internal/research/storage"
)

// researchNewsComposition 保存 News → Research 的真实模块内依赖。
// 它只在游标 Key Ring 已配置时创建；未配置时不生成默认密钥，也不开放分页查询。
type researchNewsComposition struct {
	query      *newsapp.EvidenceQueryFacade
	research   *researchapplication.Service
	importer   *researchnews.Importer
	authorizer *researchnews.Authorizer
}

// newResearchNewsComposition 在 Server 组合边界装配 News PIT 查询、当前授权复核、
// Research 持久化服务以及既有 News Adapter。所有 Repository 共用 Server 持有的 DB。
func newResearchNewsComposition(db *sql.DB, cfg newsconfig.Config) (*researchNewsComposition, error) {
	if db == nil {
		return nil, errors.New("News/Research composition requires the shared MariaDB resource")
	}
	activeKeyID, keyRing, configured := cfg.EvidenceCursorKeyRing()
	if !configured {
		return nil, errors.New("News Evidence cursor Key Ring is not configured")
	}

	newsRepository, err := newsstorage.NewMySQLRepository(db)
	if err != nil {
		return nil, fmt.Errorf("创建 News MySQL Repository 失败: %w", err)
	}
	verifier := newsplatform.NewAuthorizationFingerprintVerifier()
	authorization, err := newsapp.NewEvaluateNewsEvidenceReadAuthorization(verifier, newsRepository)
	if err != nil {
		return nil, fmt.Errorf("创建 News Evidence 当前授权复核器失败: %w", err)
	}
	getEvidence, err := newsapp.NewGetEvidence(newsRepository, newsRepository, authorization)
	if err != nil {
		return nil, fmt.Errorf("创建 News 单条 Evidence 查询失败: %w", err)
	}
	snapshotFactory, err := newsapp.NewCreateEvidenceSnapshotForQuery(
		newsRepository, newsRepository, newResearchNewsSnapshotID,
	)
	if err != nil {
		return nil, fmt.Errorf("创建 News Evidence Snapshot Factory 失败: %w", err)
	}
	cursorCodec, err := newsapp.NewEvidenceCursorCodec(activeKeyID, keyRing)
	if err != nil {
		return nil, fmt.Errorf("创建 News Evidence Cursor Codec 失败: %w", err)
	}
	snapshotPages, err := newsapp.NewListEvidenceSnapshotPage(snapshotFactory, cursorCodec)
	if err != nil {
		return nil, fmt.Errorf("创建 News Evidence Snapshot Page Reader 失败: %w", err)
	}
	listEvidence, err := newsapp.NewListEvidence(snapshotPages, getEvidence)
	if err != nil {
		return nil, fmt.Errorf("创建 News Evidence 分页查询失败: %w", err)
	}
	query, err := newsapp.NewEvidenceQueryFacade(getEvidence, listEvidence)
	if err != nil {
		return nil, fmt.Errorf("创建 News Evidence Query Facade 失败: %w", err)
	}

	contexts, err := researchstorage.NewContextRepository(db, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 Research Context Repository 失败: %w", err)
	}
	evidence, err := researchstorage.NewEvidenceRepository(db, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 Research Evidence Repository 失败: %w", err)
	}
	authorizer, err := researchnews.NewAuthorizer(query)
	if err != nil {
		return nil, fmt.Errorf("创建 Research News Authorizer 失败: %w", err)
	}
	researchService, err := researchapplication.NewService(contexts, evidence, authorizer, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("创建 Research Application Service 失败: %w", err)
	}
	importer, err := researchnews.NewImporter(query, researchService)
	if err != nil {
		return nil, fmt.Errorf("创建 Research News Evidence Importer 失败: %w", err)
	}
	return &researchNewsComposition{
		query: query, research: researchService, importer: importer, authorizer: authorizer,
	}, nil
}

func newResearchNewsSnapshotID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// 空 ID 会被 EvidenceSnapshot.Validate 拒绝；绝不退化为可预测 ID。
		return ""
	}
	return hex.EncodeToString(raw[:])
}
