package service

import "testing"

func TestValidServiceDateRange(t *testing.T) {
	cases := []struct {
		start, end string
		want bool
	}{
		{"2026-01-01", "2026-10-30", true},
		{"2026-10-30", "2026-01-01", false},
		{"20260101", "2026-10-30", false},
		{"", "2026-10-30", true},
		{"2026-01-01", "", true},
		{"", "", true},
	}
	for _, tc := range cases {
		if got := validServiceDateRange(tc.start, tc.end); got != tc.want {
			t.Errorf("validServiceDateRange(%q, %q) = %t, want %t", tc.start, tc.end, got, tc.want)
		}
	}
}
