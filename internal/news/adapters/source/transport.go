package source

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"example.com/stock-analysis-flow/internal/news/app"
)

var (
	ErrInvalidEndpointBinding    = errors.New("news: invalid source endpoint transport binding")
	ErrEndpointTransportNotBound = errors.New("news: source endpoint transport not bound")
)

// EndpointKey 是 Source/Endpoint 传输策略的唯一选择键。
type EndpointKey struct {
	SourceID   string
	EndpointID string
}

// EndpointTransport 将每个 Source Endpoint 固定映射到独立 HTTPDoer。
// 生产组合根必须注入平台 BoundDoer；不得将裸 http.Client 作为生产绑定。
type EndpointTransport struct {
	doers map[EndpointKey]app.HTTPDoer
}

// NewEndpointTransport 创建不可变的 Endpoint 传输绑定表。
// 构造时复制 map，避免调用方在运行期替换授权传输器。
func NewEndpointTransport(bindings map[EndpointKey]app.HTTPDoer) (*EndpointTransport, error) {
	if len(bindings) == 0 {
		return nil, ErrInvalidEndpointBinding
	}
	copied := make(map[EndpointKey]app.HTTPDoer, len(bindings))
	for key, doer := range bindings {
		if strings.TrimSpace(key.SourceID) == "" || strings.TrimSpace(key.EndpointID) == "" ||
			key.SourceID != strings.TrimSpace(key.SourceID) || key.EndpointID != strings.TrimSpace(key.EndpointID) || doer == nil {
			return nil, ErrInvalidEndpointBinding
		}
		copied[key] = doer
	}
	return &EndpointTransport{doers: copied}, nil
}

// Do 只调用精确匹配的 endpoint 绑定；缺失时失败关闭，不回退到默认 Client。
func (t *EndpointTransport) Do(ctx context.Context, sourceID, endpointID string, request *http.Request) (*http.Response, error) {
	if t == nil || ctx == nil || request == nil || request.URL == nil ||
		strings.TrimSpace(sourceID) == "" || strings.TrimSpace(endpointID) == "" {
		return nil, ErrInvalidEndpointBinding
	}
	key := EndpointKey{SourceID: sourceID, EndpointID: endpointID}
	doer, exists := t.doers[key]
	if !exists || doer == nil {
		return nil, ErrEndpointTransportNotBound
	}
	return doer.Do(request.Clone(ctx))
}
