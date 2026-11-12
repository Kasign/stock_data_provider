package source

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

var (
	ErrInvalidSourceCatalog = errors.New("news: invalid source catalog")
)

// StaticSourceCatalog 是启动期装载的不可变来源目录。
// 策略变更必须生成新的版本并重建目录；运行期不提供原地修改入口。
type StaticSourceCatalog struct {
	registrations map[appSourceEndpointKey]app.SourceEndpointRegistration
}

type appSourceEndpointKey struct {
	sourceID   string
	endpointID string
}

// NewStaticSourceCatalog 校验并复制来源登记快照。
// URL 与策略均来自可信启动配置，不接受采集请求动态指定。
func NewStaticSourceCatalog(registrations []app.SourceEndpointRegistration) (*StaticSourceCatalog, error) {
	if len(registrations) == 0 {
		return nil, ErrInvalidSourceCatalog
	}

	catalog := &StaticSourceCatalog{
		registrations: make(map[appSourceEndpointKey]app.SourceEndpointRegistration, len(registrations)),
	}
	for _, registration := range registrations {
		if err := validateSourceRegistration(registration); err != nil {
			return nil, errors.Join(ErrInvalidSourceCatalog, err)
		}
		key := appSourceEndpointKey{sourceID: registration.SourceID, endpointID: registration.EndpointID}
		if _, exists := catalog.registrations[key]; exists {
			return nil, ErrInvalidSourceCatalog
		}
		catalog.registrations[key] = cloneSourceRegistration(registration)
	}
	return catalog, nil
}

// LookupEndpoint 只允许精确查询已登记的 Source/Endpoint，并返回独立副本。
func (c *StaticSourceCatalog) LookupEndpoint(
	ctx context.Context,
	sourceID string,
	endpointID string,
) (app.SourceEndpointRegistration, error) {
	if c == nil || ctx == nil || !validCatalogID(sourceID) || !validCatalogID(endpointID) {
		return app.SourceEndpointRegistration{}, ErrInvalidSourceCatalog
	}
	if err := ctx.Err(); err != nil {
		return app.SourceEndpointRegistration{}, err
	}
	key := appSourceEndpointKey{sourceID: sourceID, endpointID: endpointID}
	registration, exists := c.registrations[key]
	if !exists {
		return app.SourceEndpointRegistration{}, app.ErrSourceEndpointNotFound
	}
	return cloneSourceRegistration(registration), nil
}

func validateSourceRegistration(registration app.SourceEndpointRegistration) error {
	if !validCatalogID(registration.SourceID) ||
		!validCatalogID(registration.EndpointID) ||
		!validCatalogID(registration.SourcePolicyVersionID) {
		return ErrInvalidSourceCatalog
	}
	if registration.EndpointPolicy != nil {
		if !validCatalogID(registration.EndpointPolicyVersionID) {
			return ErrInvalidSourceCatalog
		}
	} else if registration.EndpointPolicyVersionID != "" {
		return ErrInvalidSourceCatalog
	}
	if registration.MaxResponseBytes < 0 || registration.MaxResponseBytes > 16<<20 {
		return ErrInvalidSourceCatalog
	}
	if !registration.Enabled {
		return nil
	}
	if strings.TrimSpace(registration.URL) == "" ||
		registration.URL != strings.TrimSpace(registration.URL) ||
		registration.AccessMethod == "" ||
		len(registration.URLPolicy.AllowedSchemes) == 0 ||
		len(registration.URLPolicy.AllowedHosts) == 0 {
		return ErrInvalidSourceCatalog
	}
	if decision := domain.EvaluateSourceURL(registration.URL, registration.URLPolicy); !decision.Allowed {
		return errors.Join(ErrInvalidSourceCatalog, errors.New(decision.ReasonCode))
	}
	return nil
}

func validCatalogID(value string) bool {
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

func cloneSourceRegistration(value app.SourceEndpointRegistration) app.SourceEndpointRegistration {
	value.SourcePolicy.AllowedAccessMethods = append([]domain.AccessMethod(nil), value.SourcePolicy.AllowedAccessMethods...)
	value.SourcePolicy.AllowedPurposes = append([]domain.Purpose(nil), value.SourcePolicy.AllowedPurposes...)
	value.SourcePolicy.AllowedContentKinds = append([]domain.ContentKind(nil), value.SourcePolicy.AllowedContentKinds...)
	value.SourcePolicy.AllowedStorageModes = append([]domain.StorageMode(nil), value.SourcePolicy.AllowedStorageModes...)
	if value.SourcePolicy.ExpiresAt != nil {
		expiresAt := *value.SourcePolicy.ExpiresAt
		value.SourcePolicy.ExpiresAt = &expiresAt
	}

	if value.EndpointPolicy != nil {
		endpoint := *value.EndpointPolicy
		endpoint.AllowedAccessMethods = append([]domain.AccessMethod(nil), endpoint.AllowedAccessMethods...)
		endpoint.AllowedPurposes = append([]domain.Purpose(nil), endpoint.AllowedPurposes...)
		endpoint.AllowedContentKinds = append([]domain.ContentKind(nil), endpoint.AllowedContentKinds...)
		endpoint.AllowedStorageModes = append([]domain.StorageMode(nil), endpoint.AllowedStorageModes...)
		if endpoint.MaxRetention != nil {
			retention := *endpoint.MaxRetention
			endpoint.MaxRetention = &retention
		}
		if endpoint.ExpiresAt != nil {
			expiresAt := *endpoint.ExpiresAt
			endpoint.ExpiresAt = &expiresAt
		}
		value.EndpointPolicy = &endpoint
	}

	value.URLPolicy.AllowedSchemes = append([]string(nil), value.URLPolicy.AllowedSchemes...)
	value.URLPolicy.AllowedHosts = append([]string(nil), value.URLPolicy.AllowedHosts...)
	value.URLPolicy.AllowedPorts = append([]uint16(nil), value.URLPolicy.AllowedPorts...)
	return value
}

var _ app.SourceCatalog = (*StaticSourceCatalog)(nil)
