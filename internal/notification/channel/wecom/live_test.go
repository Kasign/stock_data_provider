package wecom

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
	"example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

// TestLiveWebhook is intentionally opt-in and is invoked only by the manually
// dispatched GitHub Actions workflow. It sends one visible test message.
func TestLiveWebhook(t *testing.T) {
	raw := os.Getenv("WECHAT_WORK_WEBHOOK_URL")
	if strings.TrimSpace(raw) == "" {
		if os.Getenv("WECHAT_LIVE_SMOKE") == "1" {
			t.Fatal("WECHAT_WORK_WEBHOOK_URL is required in CI")
		}
		t.Skip("WECHAT_WORK_WEBHOOK_URL is not configured")
	}
	client, err := httpclient.New(httpclient.Config{Timeout: 12 * time.Second, MaxRetries: 0})
	if err != nil {
		t.Fatal("create platform HTTP client failed")
	}
	defer client.CloseIdleConnections()
	doer, err := client.Bind(httpclient.DestinationPolicy{
		Dependency: "wecom.webhook", AllowedHosts: []string{"qyapi.weixin.qq.com"},
		AllowedPorts: []uint16{443}, AllowRedirects: false, Idempotent: false,
	})
	if err != nil {
		t.Fatal("bind WeCom destination policy failed")
	}
	secret := config.NewEnvWithLookup(func(key string) (string, bool) { return raw, key == "WECHAT_WORK_WEBHOOK_URL" }).Secret("WECHAT_WORK_WEBHOOK_URL")
	snapshot := domain.ChannelConfigSnapshot{
		ChannelRef: "wecom-live-test", ConfigVersion: 1, ChannelKind: "wecom_group_bot", Enabled: true,
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency:    domain.CapabilityUnsupported,
			SupportsDeliveryLookup: domain.CapabilityUnsupported,
			SupportsRetryAfter:     domain.CapabilityUnsupported,
			MaxPayloadBytes:        4096,
		},
	}
	adapter, err := New(snapshot, secret, doer)
	if err != nil {
		t.Fatal("validate WeCom webhook configuration failed")
	}
	request := domain.ChannelSendRequest{
		DeliveryID: "live-smoke-test",
		Target:     domain.RecipientTarget{RecipientRef: "live-test", ChannelRef: snapshot.ChannelRef, ConfigVersion: 1, AdapterTargetRef: "test-group"},
		Content: domain.RenderedContent{
			TemplateID: "wecom-live-smoke", TemplateVersion: 1,
			TemplateDigest: strings.Repeat("a", 64), EventPayloadFingerprint: strings.Repeat("b", 64),
			Format: domain.TemplateFormatText,
			Body:   "【Notification 渠道验收】这是一条自动化测试消息。验收完成后可删除临时群机器人。",
		},
	}
	result, err := adapter.Send(context.Background(), request)
	if err != nil || result.Outcome != domain.ChannelOutcomeAccepted {
		t.Fatal("WeCom live webhook smoke test failed; inspect provider status without exposing credentials")
	}
}
