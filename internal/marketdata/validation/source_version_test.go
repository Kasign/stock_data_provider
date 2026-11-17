package validation

import "testing"

func TestIsIdentifiableSourceVersion(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  bool
	}{
		{name: "provider version", value: "baostock-v1", want: true},
		{name: "trimmed provider version", value: "  hithink-2026.11  ", want: true},
		{name: "empty", value: "", want: false},
		{name: "whitespace", value: "   ", want: false},
		{name: "unknown placeholder", value: "unknown", want: false},
		{name: "case insensitive placeholder", value: " UNKNOWN ", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsIdentifiableSourceVersion(tc.value); got != tc.want {
				t.Fatalf("IsIdentifiableSourceVersion(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}
