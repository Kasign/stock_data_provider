package app

import (
	"errors"
	"testing"
	"time"
)

func TestNewsItemRelationHistoryRequestValidate(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	valid := NewsItemRelationHistoryRequest{NewsID: "news-1", SystemAsOf: now.Add(-time.Minute)}
	if err := valid.Validate(now); err != nil {
		t.Fatalf("valid request: %v", err)
	}
	cases := []struct {
		name string
		edit func(*NewsItemRelationHistoryRequest)
	}{
		{"missing news", func(r *NewsItemRelationHistoryRequest) { r.NewsID = "" }},
		{"future knowledge time", func(r *NewsItemRelationHistoryRequest) { r.SystemAsOf = now.Add(time.Second) }},
		{"non UTC knowledge time", func(r *NewsItemRelationHistoryRequest) { r.SystemAsOf = now.In(time.FixedZone("UTC+8", 8*3600)) }},
		{"zero knowledge time", func(r *NewsItemRelationHistoryRequest) { r.SystemAsOf = time.Time{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := valid
			tc.edit(&request)
			if err := request.Validate(now); !errors.Is(err, ErrInvalidNewsItemRelationHistoryQuery) {
				t.Fatalf("Validate() = %v, want invalid history query", err)
			}
		})
	}
}
