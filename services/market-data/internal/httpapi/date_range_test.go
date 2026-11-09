package httpapi

import (
	"testing"
	"time"
)

func TestInclusiveCalendarDays(t *testing.T) {
	start := time.Date(2024, time.January, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		end  time.Time
		want int
	}{
		{name: "366 days", end: start.AddDate(0, 0, 365), want: 366},
		{name: "367 days", end: start.AddDate(0, 0, 366), want: 367},
		{name: "2000 days", end: start.AddDate(0, 0, 1999), want: 2000},
		{name: "2001 days", end: start.AddDate(0, 0, 2000), want: 2001},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := inclusiveCalendarDays(start, test.end); got != test.want {
				t.Fatalf("inclusiveCalendarDays()=%d, want %d", got, test.want)
			}
		})
	}
}
