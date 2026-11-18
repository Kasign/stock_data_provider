package wecom

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
	"example.com/stock-analysis-flow/internal/platform/config"
)

type stubDoer struct {
	response *http.Response
	err error
	request *http.Request
}

func (d *stubDoer) Do(request *http.Request) (*http.Response, error) {
	d.request = request
	return d.response, d.err
}

func testSnapshot() domain.ChannelConfigSnapshot {
	return domain.ChannelConfigSnapshot{
		ChannelRef: "wecom-test", ConfigVersion: 1, ChannelKind: "wecom_group_bot", Enabled: true,
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency: domain.CapabilityUnsupported,
			SupportsDeliveryLookup: domain.CapabilityUnsupported,
			SupportsRetryAfter: domain.CapabilityUnsupported,
			MaxPayloadBytes: 4096,
		},
	}
}

func testRequest(format domain.TemplateFormat) domain.ChannelSendRequest {
	return domain.ChannelSendRequest{
		DeliveryID: "delivery-1",
		Target: domain.RecipientTarget{RecipientRef:"recipient-1", ChannelRef:"wecom-test", ConfigVersion:1, AdapterTargetRef:"group-1"},
		Content: domain.RenderedContent{
			TemplateID:"test", TemplateVersion:1,
			TemplateDigest:strings.Repeat("a",64), EventPayloadFingerprint:strings.Repeat("b",64),
			Format:format, Body:"Notification integration test",
		},
	}
}

func TestNewRejectsUntrustedWebhookDestinations(t *testing.T) {
	for _, value := range []string{
		"https://evil.example/cgi-bin/webhook/send?key=secret",
		"http://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret",
		"https://qyapi.weixin.qq.com.evil.example/cgi-bin/webhook/send?key=secret",
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=",
		"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret&x=y",
	} {
		_, err := New(testSnapshot(), config.NewEnvWithLookup(func(string)(string,bool){return value,true}).Secret("WEBHOOK"), &stubDoer{})
		if !errors.Is(err, ErrInvalidWebhookURL) {
			t.Fatalf("expected invalid webhook rejection for configured endpoint, got %v", err)
		}
	}
}

func TestSendTextAndMarkdown(t *testing.T) {
	for _, tc := range []struct{format domain.TemplateFormat; want string}{
		{domain.TemplateFormatText, `"msgtype":"text"`},
		{domain.TemplateFormatMarkdown, `"msgtype":"markdown"`},
	} {
		t.Run(string(tc.format), func(t *testing.T) {
			doer := &stubDoer{response:&http.Response{StatusCode:200, Body:io.NopCloser(strings.NewReader(`{"errcode":0,"errmsg":"ok"}`))}}
			secret := config.NewEnvWithLookup(func(string)(string,bool){return "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret-value",true}).Secret("WEBHOOK")
			adapter, err := New(testSnapshot(), secret, doer)
			if err != nil { t.Fatal(err) }
			result, err := adapter.Send(context.Background(), testRequest(tc.format))
			if err != nil || result.Outcome != domain.ChannelOutcomeAccepted { t.Fatalf("unexpected send result: %#v, %v", result, err) }
			if doer.request == nil || !strings.Contains(doer.request.URL.RawQuery, "key=secret-value") { t.Fatal("request did not target configured webhook") }
			if doer.request.Header.Get("Content-Type") != "application/json; charset=utf-8" { t.Fatal("missing JSON content type") }
			body, _ := io.ReadAll(doer.request.Body)
			if !strings.Contains(string(body), tc.want) { t.Fatalf("unexpected payload: %s", body) }
		})
	}
}

func TestSendClassifiesProviderOutcomes(t *testing.T) {
	cases := []struct{name string; response *http.Response; transportErr error; outcome domain.ChannelSendOutcome}{
		{"provider_rejection",&http.Response{StatusCode:200,Body:io.NopCloser(strings.NewReader(`{"errcode":40001,"errmsg":"invalid"}`))},nil,domain.ChannelOutcomeRejected},
		{"http_error",&http.Response{StatusCode:503,Body:io.NopCloser(strings.NewReader("unavailable"))},nil,domain.ChannelOutcomeAmbiguous},
		{"transport_error",nil,errors.New("network failure"),domain.ChannelOutcomeAmbiguous},
		{"malformed_response",&http.Response{StatusCode:200,Body:io.NopCloser(strings.NewReader("not-json"))},nil,domain.ChannelOutcomeAmbiguous},
	}
	for _, tc := range cases { t.Run(tc.name,func(t *testing.T){
		doer:=&stubDoer{response:tc.response,err:tc.transportErr}
		secret:=config.NewEnvWithLookup(func(string)(string,bool){return "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=secret",true}).Secret("WEBHOOK")
		adapter,err:=New(testSnapshot(),secret,doer); if err!=nil {t.Fatal(err)}
		result,sendErr:=adapter.Send(context.Background(),testRequest(domain.TemplateFormatText))
		if result.Outcome!=tc.outcome || sendErr==nil {t.Fatalf("got %#v, %v",result,sendErr)}
	})}
}

func TestAdapterErrorsDoNotContainWebhookSecret(t *testing.T) {
	doer:=&stubDoer{err:errors.New("transport detail")}
	secretValue:="top-secret-webhook-key"
	secret:=config.NewEnvWithLookup(func(string)(string,bool){return "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key="+secretValue,true}).Secret("WEBHOOK")
	adapter,err:=New(testSnapshot(),secret,doer); if err!=nil {t.Fatal(err)}
	_,sendErr:=adapter.Send(context.Background(),testRequest(domain.TemplateFormatText))
	if sendErr==nil || strings.Contains(sendErr.Error(),secretValue) {t.Fatalf("secret leaked in error: %v",sendErr)}
}
