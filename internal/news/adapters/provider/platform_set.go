package provider

import (
	"example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

// NewPlatformProviderSet 根据已校验的 News 配置创建 Provider，并为每个 Provider
// endpoint 单独绑定 Platform DestinationPolicy。共享 Client 生命周期仍归组合根。
func NewPlatformProviderSet(config newsconfig.Config, client *httpclient.Client) (app.ProviderSet, error) {
	providers := make([]app.Provider, 0, 4)

	if value := config.Bocha(); value.Configured() {
		doer, err := NewPlatformHTTPDoer(client, value.Endpoint(), "news.provider.bocha")
		if err != nil {
			return nil, app.ErrProviderConfig
		}
		provider, err := NewBocha(value.Endpoint(), value.APIKey().Value(), doer)
		if err != nil {
			return nil, app.ErrProviderConfig
		}
		providers = append(providers, provider)
	}
	if value := config.Tavily(); value.Configured() {
		doer, err := NewPlatformHTTPDoer(client, value.Endpoint(), "news.provider.tavily")
		if err != nil {
			return nil, app.ErrProviderConfig
		}
		provider, err := NewTavily(value.Endpoint(), value.APIKey().Value(), doer)
		if err != nil {
			return nil, app.ErrProviderConfig
		}
		providers = append(providers, provider)
	}
	if value := config.Brave(); value.Configured() {
		doer, err := NewPlatformHTTPDoer(client, value.Endpoint(), "news.provider.brave")
		if err != nil {
			return nil, app.ErrProviderConfig
		}
		provider, err := NewBrave(value.Endpoint(), value.APIKey().Value(), doer)
		if err != nil {
			return nil, app.ErrProviderConfig
		}
		providers = append(providers, provider)
	}
	if value := config.SearXNG(); value.Configured() {
		doer, err := NewPlatformHTTPDoer(client, value.Endpoint(), "news.provider.searxng")
		if err != nil {
			return nil, app.ErrProviderConfig
		}
		provider, err := NewSearXNG(value.Endpoint(), doer)
		if err != nil {
			return nil, app.ErrProviderConfig
		}
		providers = append(providers, provider)
	}

	return app.StaticProviderSet{Providers: providers}, nil
}
