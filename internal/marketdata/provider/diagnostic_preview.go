package provider

import (
	"unicode/utf8"
)

// truncateDiagnosticPreview 按字节预算截断诊断预览，但不切断 UTF-8 多字节字符。
func truncateDiagnosticPreview(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
