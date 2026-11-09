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

var ErrRepositoryNotFound = errors.New("notification: template repository record not found")
