package provider

import (
	"strings"
	"testing"
	"unicode/utf8"
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
