package source

import (
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

// NewPlatformEndpointTransport 为已登记且支持 RSS/Atom 的 Source Endpoint
// 分别创建 BoundDoer。调用方应传入可信 SourceCatalog 的启动快照；
// 后续注册表变更必须重新构建传输绑定，不能退回共享裸 Client。
func NewPlatformEndpointTransport(
	client *httpclient.Client,
	registrations []app.SourceEndpointRegistration,
) (*EndpointTransport, error) {
	if client == nil || len(registrations) == 0 {
		return nil, ErrInvalidEndpointBinding
	}

	bindings := make(map[EndpointKey]app.HTTPDoer, len(registrations))
	for _, registration := range registrations {
		if !registration.Enabled ||
			(registration.AccessMethod != domain.AccessRSS && registration.AccessMethod != domain.AccessLicensedFeed) {
			continue
		}
		key := EndpointKey{SourceID: registration.SourceID, EndpointID: registration.EndpointID}
		if _, exists := bindings[key]; exists {
			return nil, ErrInvalidEndpointBinding
		}
		dependency := sourceDependency(registration.SourceID, registration.EndpointID)
		doer, err := NewPlatformHTTPDoer(client, registration.URL, dependency)
		if err != nil {
			return nil, err
		}
		bindings[key] = doer
	}
	if len(bindings) == 0 {
		return nil, ErrInvalidEndpointBinding
	}
	return NewEndpointTransport(bindings)
}

func sourceDependency(sourceID, endpointID string) string {
	return fmt.Sprintf("news.source.%s.%s", strings.TrimSpace(sourceID), strings.TrimSpace(endpointID))
}
