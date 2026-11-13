package provider

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

var ErrInvalidOutputPolicyCatalog = errors.New("news: invalid provider output policy catalog")

// StaticOutputPolicyCatalog 是启动期装载的不可变 Provider 输出授权目录。
// 每次授权变更必须提升 PolicyVersion 并重建目录，禁止原地修改已发布策略。
type StaticOutputPolicyCatalog struct {
	bindings map[string]app.ProviderOutputPolicyBinding
}

// NewStaticOutputPolicyCatalog 校验并复制可信 Provider 输出策略快照。
func NewStaticOutputPolicyCatalog(bindings []app.ProviderOutputPolicyBinding) (*StaticOutputPolicyCatalog, error) {
	if len(bindings) == 0 {
		return nil, ErrInvalidOutputPolicyCatalog
	}
	catalog := &StaticOutputPolicyCatalog{bindings: make(map[string]app.ProviderOutputPolicyBinding, len(bindings))}
	for _, binding := range bindings {
		if !validPolicyCatalogID(binding.ProviderCode) ||
			!validPolicyCatalogID(binding.PolicyRef) ||
			!validPolicyCatalogID(binding.PolicyVersion) ||
			binding.Policy.MaxRequestRetention < 0 ||
			binding.Policy.MaxCacheRetention < 0 ||
			binding.Policy.MaxPersistenceRetention < 0 {
			return nil, ErrInvalidOutputPolicyCatalog
		}
		if _, exists := catalog.bindings[binding.ProviderCode]; exists {
			return nil, ErrInvalidOutputPolicyCatalog
		}
		catalog.bindings[binding.ProviderCode] = cloneOutputPolicyBinding(binding)
	}
	return catalog, nil
}

// ResolveProviderOutputPolicy 只返回启动时登记的精确 Provider 策略版本。
func (c *StaticOutputPolicyCatalog) ResolveProviderOutputPolicy(
	ctx context.Context,
	providerCode string,
) (app.ProviderOutputPolicyBinding, error) {
	if c == nil || ctx == nil || !validPolicyCatalogID(providerCode) {
		return app.ProviderOutputPolicyBinding{}, ErrInvalidOutputPolicyCatalog
	}
	if err := ctx.Err(); err != nil {
		return app.ProviderOutputPolicyBinding{}, err
	}
	binding, exists := c.bindings[providerCode]
	if !exists {
		return app.ProviderOutputPolicyBinding{}, app.ErrProviderOutputPolicyUnavailable
	}
	return cloneOutputPolicyBinding(binding), nil
}

func validPolicyCatalogID(value string) bool {
	if value == "" || value != strings.TrimSpace(value) || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if unicode.IsControl(char) || unicode.IsSpace(char) || char > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func cloneOutputPolicyBinding(value app.ProviderOutputPolicyBinding) app.ProviderOutputPolicyBinding {
	value.Policy.AllowedPurposes = append([]domain.Purpose(nil), value.Policy.AllowedPurposes...)
	value.Policy.AllowedContentKinds = append([]domain.ContentKind(nil), value.Policy.AllowedContentKinds...)
	value.Policy.AllowedStorageModes = append([]domain.StorageMode(nil), value.Policy.AllowedStorageModes...)
	if value.Policy.ExpiresAt != nil {
		expiresAt := *value.Policy.ExpiresAt
		value.Policy.ExpiresAt = &expiresAt
	}
	return value
}

var _ app.ProviderOutputPolicyResolver = (*StaticOutputPolicyCatalog)(nil)
