package template

import (
	"context"
	"errors"
	"strings"
)

// Repository 保存不可变模板定义版本。相同键与相同 Digest 可幂等写入；内容不同必须返回 ErrTemplateImmutable。
type Repository interface {
	Create(ctx context.Context, definition Definition) (digest string, duplicate bool, err error)
	Get(ctx context.Context, templateID string, version int64) (definition Definition, digest string, found bool, err error)
}

var (
	ErrRepositoryNotFound = errors.New("notification: template repository record not found")
	ErrRepositoryUnavailable = errors.New("notification: template repository is unavailable")
	ErrRepositoryDigestMismatch = errors.New("notification: template repository digest mismatch")
)

// LoadFromRepository 按指定不可变版本装载模板；不回退 latest，也不允许覆盖已装载的不同定义。
func (r *Registry) LoadFromRepository(ctx context.Context, repository Repository, templateID string, version int64) error {
	if r == nil {
		return ErrInvalidTemplateLimits
	}
	if ctx == nil || repository == nil || strings.TrimSpace(templateID) == "" || version <= 0 {
		return ErrRepositoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	definition, storedDigest, found, err := repository.Get(ctx, templateID, version)
	if err != nil {
		return err
	}
	if !found {
		return ErrUnknownTemplate
	}
	if definition.TemplateID != templateID || definition.Version != version {
		return ErrRepositoryDigestMismatch
	}
	actualDigest, err := DefinitionDigest(definition)
	if err != nil {
		return err
	}
	if storedDigest == "" || actualDigest != storedDigest {
		return ErrRepositoryDigestMismatch
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	registeredDigest, err := r.Register(definition)
	if err != nil {
		return err
	}
	if registeredDigest != storedDigest {
		return ErrRepositoryDigestMismatch
	}
	return nil
}
