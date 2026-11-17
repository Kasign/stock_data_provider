package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrNewsPolicyVersionConflict = errors.New("news.policy.version_conflict")
	ErrNewsPolicyVersionStore    = errors.New("news.policy.version_store_failed")
)

// NewsPolicyVersionWriter 原子保存不可变版本并切换当前指针。
type NewsPolicyVersionWriter interface {
	PublishNewsPolicyVersion(context.Context, domain.NewsPolicyVersion) error
}

type PublishNewsPolicyVersion struct {
	writer NewsPolicyVersionWriter
	now    func() time.Time
}

func NewPublishNewsPolicyVersion(writer NewsPolicyVersionWriter) (*PublishNewsPolicyVersion, error) {
	if writer == nil {
		return nil, ErrNewsPolicyVersionStore
	}
	return &PublishNewsPolicyVersion{writer: writer, now: time.Now}, nil
}

// Execute 发布一个策略版本。相同身份/版本和相同载荷可幂等重试；载荷不同必须冲突。
func (u *PublishNewsPolicyVersion) Execute(
	ctx context.Context,
	policyType, policyRef, policyVersion string,
	payload []byte,
) error {
	if u == nil || u.writer == nil || u.now == nil || ctx == nil {
		return ErrNewsPolicyVersionStore
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	sum := sha256.Sum256(payload)
	value := domain.NewsPolicyVersion{
		PolicyType: policyType, PolicyRef: policyRef, PolicyVersion: policyVersion,
		Payload: append([]byte(nil), payload...), PayloadHash: hex.EncodeToString(sum[:]),
		CreatedAt: u.now().UTC(),
	}
	if err := domain.ValidateNewsPolicyVersion(value); err != nil {
		return errors.Join(ErrNewsPolicyVersionStore, err)
	}
	if err := u.writer.PublishNewsPolicyVersion(ctx, value); err != nil {
		return err
	}
	return ctx.Err()
}
