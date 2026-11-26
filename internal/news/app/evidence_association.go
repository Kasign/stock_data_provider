package app

import (
	"context"
	"errors"
)

var (
	// ErrEvidenceAssociationNotFound 表示请求的 Evidence 身份与该 Run 已登记
	// Stage Output 之间没有精确关联。这是正常的拒绝，不是数据损坏。
	ErrEvidenceAssociationNotFound = errors.New("news.evidence.association_not_found")
	// ErrEvidenceAssociationUnavailable 表示关联校验的依赖不可用，必须 fail-closed，
	// 不得降级为"无关联"或放行。
	ErrEvidenceAssociationUnavailable = errors.New("news.evidence.association_unavailable")
)

// RunEvidenceAssociationVerifier 只证明某个 Evidence 身份确实属于指定 Run。
//
// 它不参与授权判定、不读取正文、不触碰 News 的当前策略锁表；授权与 PIT 读取
// 仍完全由 GetEvidence.Execute 负责。runRef 必须由可信的进程内执行边界注入，
// 不得来自 HTTP 请求体、Query 或自定义 Header。
//
// 本接口刻意与 Quant 的 ResultAssociationVerifier 分离：两者的身份模型
// （内容寻址 result_id 对 NewsID/VersionID）、策略锁表和授权主体都不同。
type RunEvidenceAssociationVerifier interface {
	VerifyEvidenceAssociation(ctx context.Context, runRef string, request GetEvidenceRequest) error
}
