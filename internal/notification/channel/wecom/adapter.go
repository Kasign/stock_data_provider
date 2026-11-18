// Package wecom implements the WeCom group-bot webhook channel.
package wecom

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"example.com/stock-analysis-flow/internal/notification/domain"
	"example.com/stock-analysis-flow/internal/platform/config"
)

const (
	webhookHost = "qyapi.weixin.qq.com"
	webhookPath = "/cgi-bin/webhook/send"
	maxResponseBytes = 64 << 10
)

var (
	ErrInvalidWebhookURL = errors.New("notification: invalid WeCom webhook configuration")
	ErrUnsupportedFormat = errors.New("notification: unsupported WeCom message format")
)

type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

type Adapter struct {
	snapshot domain.ChannelConfigSnapshot
	webhook  config.Secret
	doer     Doer
}

func New(snapshot domain.ChannelConfigSnapshot, webhook config.Secret, doer Doer) (*Adapter, error) {
	if err := snapshot.Validate(); err != nil {
		return nil, err
	}
	if snapshot.ChannelKind != "wecom_group_bot" || !snapshot.Enabled ||
		!webhook.Configured() || doer == nil {
		return nil, ErrInvalidWebhookURL
	}
	if _, err := parseWebhookURL(webhook.Value()); err != nil {
		return nil, err
	}
	return &Adapter{snapshot: snapshot, webhook: webhook, doer: doer}, nil
}

func (a *Adapter) ConfigSnapshot() domain.ChannelConfigSnapshot {
	snapshot := a.snapshot
	snapshot.RetryableErrorCodes = append([]string(nil), a.snapshot.RetryableErrorCodes...)
	return snapshot
}

func (a *Adapter) Send(ctx context.Context, request domain.ChannelSendRequest) (domain.ChannelSendResult, error) {
	if a == nil || a.doer == nil || ctx == nil {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "invalid_adapter"}, errors.New("notification: WeCom adapter unavailable")
	}
	if err := request.Validate(a.snapshot); err != nil {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "invalid_request"}, err
	}
	payload, err := buildPayload(request.Content)
	if err != nil {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "unsupported_format"}, err
	}
	endpoint, err := parseWebhookURL(a.webhook.Value())
	if err != nil {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "invalid_configuration"}, ErrInvalidWebhookURL
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "encode_failed"}, errors.New("notification: encode WeCom message failed")
	}
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "request_build_failed"}, errors.New("notification: build WeCom request failed")
	}
	httpRequest.Header.Set("Content-Type", "application/json; charset=utf-8")
	response, err := a.doer.Do(httpRequest)
	if err != nil {
		// A transport error cannot prove whether the provider accepted the message.
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAmbiguous, ErrorCode: "transport_ambiguous"}, errors.New("notification: WeCom send outcome is ambiguous")
	}
	if response == nil || response.Body == nil {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAmbiguous, ErrorCode: "empty_response"}, errors.New("notification: WeCom returned no response body")
	}
	defer response.Body.Close()
	responseBody, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if readErr != nil || len(responseBody) > maxResponseBytes {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAmbiguous, ErrorCode: "invalid_response"}, errors.New("notification: read WeCom response failed")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAmbiguous, ErrorCode: "http_" + strconv.Itoa(response.StatusCode)}, fmt.Errorf("notification: WeCom returned HTTP status %d", response.StatusCode)
	}
	var providerResult struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.Unmarshal(responseBody, &providerResult); err != nil {
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAmbiguous, ErrorCode: "invalid_provider_response"}, errors.New("notification: decode WeCom response failed")
	}
	if providerResult.ErrCode != 0 {
		code := "wecom.errcode." + strconv.Itoa(providerResult.ErrCode)
		return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeRejected, ErrorCode: code}, fmt.Errorf("notification: WeCom rejected message with code %d", providerResult.ErrCode)
	}
	return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAccepted}, nil
}

type messagePayload struct {
	MsgType  string          `json:"msgtype"`
	Text     *textContent    `json:"text,omitempty"`
	Markdown *textContent    `json:"markdown,omitempty"`
}

type textContent struct {
	Content string `json:"content"`
}

func buildPayload(content domain.RenderedContent) (messagePayload, error) {
	if strings.TrimSpace(content.Body) == "" {
		return messagePayload{}, ErrUnsupportedFormat
	}
	switch content.Format {
	case domain.TemplateFormatText:
		return messagePayload{MsgType: "text", Text: &textContent{Content: content.Body}}, nil
	case domain.TemplateFormatHTML:
		return messagePayload{MsgType: "markdown", Markdown: &textContent{Content: content.Body}}, nil
	default:
		return messagePayload{}, ErrUnsupportedFormat
	}
}

func parseWebhookURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil || parsed.Scheme != "https" ||
		!strings.EqualFold(parsed.Hostname(), webhookHost) ||
		parsed.Port() != "" || parsed.User != nil || parsed.Fragment != "" ||
		parsed.Path != webhookPath || parsed.RawPath != "" {
		return nil, ErrInvalidWebhookURL
	}
	query := parsed.Query()
	if len(query) != 1 || strings.TrimSpace(query.Get("key")) == "" || len(query.Get("key")) > 256 {
		return nil, ErrInvalidWebhookURL
	}
	for key := range query {
		if key != "key" {
			return nil, ErrInvalidWebhookURL
		}
	}
	return parsed, nil
}
