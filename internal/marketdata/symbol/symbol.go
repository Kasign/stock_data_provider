package symbol

import (
	"fmt"
	"regexp"
	"strings"
)

var pattern = regexp.MustCompile("^[0-9]{6}(\\.(SH|SZ|BJ))?$")

// Normalize validates an A-share symbol and returns its canonical exchange suffix.
func Normalize(raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if !pattern.MatchString(value) {
		return "", fmt.Errorf("invalid A-share symbol %q", raw)
	}
	parts := strings.Split(value, ".")
	code := parts[0]
	// Shanghai 900xxx and Shenzhen 200xxx are B-shares outside this data contract.
	if strings.HasPrefix(code, "900") || strings.HasPrefix(code, "200") {
		return "", fmt.Errorf("B-share code %q is outside the supported A-share data contract", code)
	}
	exchange := ""
	switch {
	case strings.HasPrefix(code, "92"), strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"):
		exchange = "BJ"
	case strings.HasPrefix(code, "6"), strings.HasPrefix(code, "5"), strings.HasPrefix(code, "9"):
		exchange = "SH"
	case strings.HasPrefix(code, "0"), strings.HasPrefix(code, "1"), strings.HasPrefix(code, "2"), strings.HasPrefix(code, "3"):
		exchange = "SZ"
	default:
		return "", fmt.Errorf("cannot infer exchange for %q", raw)
	}
	if len(parts) == 2 && parts[1] != exchange {
		return "", fmt.Errorf("symbol %q has exchange suffix %s but code prefix maps to %s", raw, parts[1], exchange)
	}
	return code + "." + exchange, nil
}
