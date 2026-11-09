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
	ErrRepositoryNotFound       = errors.New("notification: template repository record not found")
	ErrRepositoryUnavailable    = errors.New("notification: template repository is unavailable")
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

// VersionRef 标识需要在启动阶段精确装载的模板版本。
type VersionRef struct {
	TemplateID string
	Version    int64
}

// LoadVersionsFromRepository 批量装载精确版本。所有记录校验和编译成功后才一次性写入 Registry，避免部分装载。
func (r *Registry) LoadVersionsFromRepository(ctx context.Context, repository Repository, versions []VersionRef) error {
	if r == nil {
		return ErrInvalidTemplateLimits
	}
	if ctx == nil || repository == nil || len(versions) == 0 {
		return ErrRepositoryUnavailable
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	type preparedVersion struct {
		key      templateKey
		compiled *compiledTemplate
		digest   string
	}
	prepared := make([]preparedVersion, 0, len(versions))
	seen := make(map[templateKey]struct{}, len(versions))
	for _, version := range versions {
		if strings.TrimSpace(version.TemplateID) == "" || version.Version <= 0 {
			return ErrRepositoryUnavailable
		}
		key := templateKey{id: version.TemplateID, version: version.Version}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("%w: duplicate template version %s/%d", ErrRepositoryDigestMismatch, version.TemplateID, version.Version)
		}
		seen[key] = struct{}{}

		definition, storedDigest, found, err := repository.Get(ctx, version.TemplateID, version.Version)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("%w: %s/%d", ErrUnknownTemplate, version.TemplateID, version.Version)
		}
		if definition.TemplateID != version.TemplateID || definition.Version != version.Version {
			return ErrRepositoryDigestMismatch
		}
		digest, err := DefinitionDigest(definition)
		if err != nil {
			return err
		}
		if storedDigest == "" || digest != storedDigest {
			return ErrRepositoryDigestMismatch
		}
		if err := ValidateDefinition(definition, r.limits); err != nil {
			return err
		}
		compiled, err := compileDefinition(definition, digest)
		if err != nil {
			return err
		}
		prepared = append(prepared, preparedVersion{key: key, compiled: compiled, digest: digest})
		if err := ctx.Err(); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, item := range prepared {
		if existing, exists := r.templates[item.key]; exists && existing.digest != item.digest {
			return ErrTemplateImmutable
		}
	}
	for _, item := range prepared {
		if _, exists := r.templates[item.key]; !exists {
			r.templates[item.key] = item.compiled
		}
	}
	return nil
}
