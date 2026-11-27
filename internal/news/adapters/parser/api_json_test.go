package parser

import (
	"errors"
	"testing"
	"time"
)

// 真实 cninfo hisAnnouncement/query 响应结构的最小可信样本（字段值来自 1.19 节实测）。
const cninfoSample = `{
  "totalAnnouncement": 2,
  "announcements": [
    {
      "announcementId": "1225649440",
      "announcementTitle": "[H股公告]招商银行股份有限公司更换联席公司秘书",
      "announcementTime": 1793376000000,
      "adjunctUrl": "finalpage/2026-10-31/1225649440.PDF",
      "orgId": "gssh0600036",
      "storageTime": null
    },
    {
      "announcementId": "1225649239",
      "announcementTitle": "招商银行股份有限公司2026年第三季度报告",
      "announcementTime": 1793376000000,
      "adjunctUrl": "finalpage/2026-10-31/1225649239.PDF",
      "orgId": "gssh0600036"
    }
  ]
}`

func TestDetectOfficialAPIFormatAcceptsApplicationJSONOnly(t *testing.T) {
	for _, value := range []string{"application/json", "application/json; charset=utf-8", "APPLICATION/JSON"} {
		got, err := DetectOfficialAPIFormat(value)
		if err != nil || got != FeedJSON {
			t.Fatalf("content_type=%q got=%q err=%v", value, got, err)
		}
	}
	for _, value := range []string{"", "text/html", "application/rss+xml", "text/plain", "application/json-seq", "invalid"} {
		if _, err := DetectOfficialAPIFormat(value); !errors.Is(err, ErrUnsupportedFeedContentType) {
			t.Fatalf("content_type=%q err=%v", value, err)
		}
	}
}

func TestParseAPIJSONKeepsDisclosureRawValueWithoutParsingTime(t *testing.T) {
	observedAt := time.Date(2026, 11, 27, 8, 29, 0, 0, time.UTC)
	doc, err := ParseAPIJSON([]byte(cninfoSample), observedAt)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != FeedJSON {
		t.Fatalf("format=%q want %q", doc.Format, FeedJSON)
	}
	if !doc.ObservedAt.Equal(observedAt) {
		t.Fatalf("observed_at=%v want %v", doc.ObservedAt, observedAt)
	}
	if len(doc.Entries) != 2 {
		t.Fatalf("entries=%d want 2", len(doc.Entries))
	}
	first := doc.Entries[0]
	if first.ID != "1225649440" {
		t.Fatalf("id=%q", first.ID)
	}
	if first.Title == "" {
		t.Fatal("title must be preserved")
	}
	if first.Link != "finalpage/2026-10-31/1225649440.PDF" {
		t.Fatalf("link=%q", first.Link)
	}
	// announcementTime 是披露日编码，必须原样保留为毫秒字符串，不得解析为精确时间。
	if first.PublishedAtRaw != "1793376000000" {
		t.Fatalf("published_at_raw=%q want raw millis", first.PublishedAtRaw)
	}
	if first.PublishedAt != nil {
		t.Fatalf("published_at must stay nil for disclosure-date field, got=%v", *first.PublishedAt)
	}
}

func TestParseAPIJSONRejectsNonNumericTime(t *testing.T) {
	sample := `{"announcements":[{"announcementId":"1","announcementTitle":"x","announcementTime":"1793376000000","adjunctUrl":"a.pdf"}]}`
	if _, err := ParseAPIJSON([]byte(sample), time.Now()); !errors.Is(err, ErrInvalidFeed) {
		t.Fatalf("string announcementTime must be rejected, err=%v", err)
	}
}

func TestParseAPIJSONRejectsMalformedAndWrongShape(t *testing.T) {
	for name, payload := range map[string]string{
		"malformed":      `{not json`,
		"array-root":     `[]`,
		"anns-not-array": `{"announcements":{}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseAPIJSON([]byte(payload), time.Now()); !errors.Is(err, ErrInvalidFeed) {
				t.Fatalf("%s err=%v", name, err)
			}
		})
	}
}

func TestParseAPIJSONSkipsEntriesWithoutID(t *testing.T) {
	sample := `{"announcements":[
		{"announcementTitle":"no id","announcementTime":1793376000000,"adjunctUrl":"a.pdf"},
		{"announcementId":"2","announcementTitle":"valid","announcementTime":1793376000000,"adjunctUrl":"b.pdf"}
	]}`
	doc, err := ParseAPIJSON([]byte(sample), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Entries) != 1 || doc.Entries[0].ID != "2" {
		t.Fatalf("entries=%+v want only valid id", doc.Entries)
	}
}

func TestParseAPIJSONAllowsEmptyAnnouncements(t *testing.T) {
	doc, err := ParseAPIJSON([]byte(`{"announcements":[]}`), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if doc.Format != FeedJSON || len(doc.Entries) != 0 {
		t.Fatalf("doc=%+v", doc)
	}
}
