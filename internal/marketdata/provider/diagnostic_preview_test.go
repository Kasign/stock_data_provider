package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"unicode/utf8"

	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestDiagnosticPreviewTruncationPreservesUTF8(t *testing.T) {
	input := strings.Repeat("a", 159) + "中" + strings.Repeat("b", 10)
	for name, got := range map[string]string{"quote": quoteIssuePreview(input), "fundamental": providerFieldIssuePreview(input)} {
		if len(got) > 160 { t.Fatalf("%s preview exceeds byte budget: %d", name, len(got)) }
		if !utf8.ValidString(got) { t.Fatalf("%s preview contains invalid UTF-8: %q", name, got) }
		if got != strings.Repeat("a", 159) { t.Fatalf("%s preview split a multibyte rune: %q", name, got) }
	}
}

func TestDiagnosticPreviewTruncationHandlesShortAndNonPositiveBudgets(t *testing.T) {
	if got := truncateDiagnosticPreview("诊断", 10); got != "诊断" { t.Fatalf("short preview changed: %q", got) }
	if got := truncateDiagnosticPreview("diagnostic", 0); got != "" { t.Fatalf("zero byte budget should return empty preview, got %q", got) }
}

func TestEastmoneyQuoteDiagnosticPreviewPreservesUTF8(t *testing.T) {
	input := strings.Repeat("a", 159) + "中" + strings.Repeat("b", 10)
	reporter := validation.NewReporter(4)
	ctx := validation.WithReporter(context.Background(), reporter)
	_, err := parseEastmoneyQuoteNumber(ctx, map[string]json.RawMessage{
		"f2": json.RawMessage(`"` + input + `"`),
	}, "f2", "close", true)
	if err == nil { t.Fatal("invalid numeric field should be rejected") }
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 { t.Fatalf("expected one diagnostic issue, got %+v", issues) }
	preview := issues[0].ActualPreview
	if len(preview) > 160 || !utf8.ValidString(preview) {
		t.Fatalf("Eastmoney preview must be valid UTF-8 within 160 bytes, got len=%d value=%q", len(preview), preview)
	}
	if preview != strings.Repeat("a", 159) { t.Fatalf("unexpected safe preview boundary: %q", preview) }
}

func TestTickFlowHTTPErrorSummaryPreservesUTF8(t *testing.T) {
	body := strings.Repeat("a", 2047) + "中" + strings.Repeat("b", 20)
	provider := &TickFlow{apiKey: "secret"}
	err := provider.tickflowHTTPStatusError(&http.Response{
		StatusCode: http.StatusBadGateway,
		Body: io.NopCloser(strings.NewReader(body)),
	})
	if err == nil { t.Fatal("HTTP error response should produce an error") }
	if !utf8.ValidString(err.Error()) { t.Fatalf("HTTP error summary is invalid UTF-8: %q", err.Error()) }
	if !strings.Contains(err.Error(), "[truncated]") { t.Fatalf("expected explicit truncation marker, got %q", err.Error()) }
}
