package parser

import (
	"errors"
	"testing"
)

func TestDetectFeedFormatUsesExplicitContentType(t *testing.T) {
	cases := []struct {
		contentType string
		want        FeedFormat
	}{
		{"application/rss+xml; charset=utf-8", FeedRSS},
		{"Application/Atom+XML", FeedAtom},
	}
	for _, tc := range cases {
		got, err := DetectFeedFormat(tc.contentType)
		if err != nil || got != tc.want {
			t.Fatalf("content_type=%q got=%q err=%v", tc.contentType, got, err)
		}
	}
	for _, value := range []string{"", "text/xml", "application/xml", "text/html", "application/json", "invalid"} {
		if _, err := DetectFeedFormat(value); !errors.Is(err, ErrUnsupportedFeedContentType) {
			t.Fatalf("content_type=%q err=%v", value, err)
		}
	}
}
