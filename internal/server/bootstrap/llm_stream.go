package bootstrap

import (
	"context"
	"errors"
	"net/http"

	"example.com/stock-analysis-flow/internal/llm"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

// platformLLMStreamOpener 是 Platform BoundDoer.OpenStream 的最窄组合根接口。
// 目标策略仍由 BoundDoer 固定，LLM 不接触 DestinationPolicy。
type platformLLMStreamOpener interface {
	OpenStream(context.Context, *http.Request) (*httpclient.StreamResponse, error)
}

type platformLLMStreamDoer struct {
	opener platformLLMStreamOpener
}

func newPlatformLLMStreamDoer(opener platformLLMStreamOpener) (llm.HTTPStreamDoer, error) {
	if opener == nil {
		return nil, errors.New("Platform-bound LLM stream opener is required")
	}
	return platformLLMStreamDoer{opener: opener}, nil
}

func (d platformLLMStreamDoer) OpenStream(
	ctx context.Context,
	request *http.Request,
) (*llm.HTTPStreamResponse, error) {
	if d.opener == nil {
		return nil, errors.New("Platform-bound LLM stream opener is not configured")
	}
	response, err := d.opener.OpenStream(ctx, request)
	if response == nil {
		return nil, err
	}
	// Body 所有权从 Platform StreamResponse 转移给 LLM Adapter；
	// 即使 OpenStream 同时返回 response 和 error，也保留 Body 让 Adapter 关闭。
	return &llm.HTTPStreamResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Body:       response.Body,
	}, err
}

var _ llm.HTTPStreamDoer = platformLLMStreamDoer{}
