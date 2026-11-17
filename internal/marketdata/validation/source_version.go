package validation

import "strings"

// IsIdentifiableSourceVersion rejects empty values and placeholders that cannot
// identify the provider schema or mapping used to produce a snapshot.
func IsIdentifiableSourceVersion(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.EqualFold(value, "unknown")
}
