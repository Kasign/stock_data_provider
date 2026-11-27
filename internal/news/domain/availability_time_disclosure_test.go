package domain

import (
	"strconv"
	"testing"
	"time"
)

// 真实巨潮样本（2026-11-27 实测）：000001 平安银行公告
// announcementTime = 1794931200000 = 北京时间 2026-11-18 00:00:00
// 该字段是"披露日"编码，不是精确首次公开时刻。
func TestParseDisclosureDateConservativeBoundary(t *testing.T) {
	got := ParseDisclosureDateConservativeBoundary("1794931200000", "+08:00")
	// 保守边界 = 披露日当天最后一秒 = 北京 2026-11-18 23:59:59 = UTC 2026-11-18 15:59:59Z
	want := time.Date(2026, 11, 18, 15, 59, 59, 0, time.UTC)
	if got.Status != AvailabilityTimeParsed ||
		got.Precision != AvailabilityTimePrecisionDisclosureBoundary ||
		got.CandidateAvailableAt == nil ||
		!got.CandidateAvailableAt.Equal(want) ||
		got.CandidateAvailableAt.Nanosecond() != 0 ||
		got.CandidateAvailableAt.Location() != time.UTC ||
		got.SourceTimezoneOrOffset != "+08:00" ||
		got.RawValue != "1794931200000" ||
		got.ReasonCode != "" {
		t.Fatalf("unexpected conservative boundary candidate: %+v", got)
	}
}

// 保守边界必须严格晚于源时间戳时刻，避免把"披露日零点"当作可获取时刻造成前视偏差。
func TestDisclosureDateBoundaryIsNeverEarlierThanInstant(t *testing.T) {
	raw := "1794931200000" // 北京 2026-11-18 00:00:00 = UTC 2026-11-17T16:00:00Z
	instant := time.UnixMilli(1794931200000).UTC()
	got := ParseDisclosureDateConservativeBoundary(raw, "+08:00")
	if got.CandidateAvailableAt == nil || !got.CandidateAvailableAt.After(instant) {
		t.Fatalf("conservative boundary must be after the source instant: got %+v, instant %s", got, instant)
	}
}

// 负偏移时区同样按"披露日当天最后一秒"推导。
func TestParseDisclosureDateConservativeBoundaryNegativeOffset(t *testing.T) {
	// 2026-11-18T02:00:00Z 在 -05:00 时区是 2026-11-17 21:00:00，披露日 = 2026-11-17
	// 边界 = 2026-11-17 23:59:59 -05:00 = UTC 2026-11-18 04:59:59Z
	ms := time.Date(2026, 11, 18, 2, 0, 0, 0, time.UTC).UnixMilli()
	got := ParseDisclosureDateConservativeBoundary(strconv.FormatInt(ms, 10), "-05:00")
	want := time.Date(2026, 11, 18, 4, 59, 59, 0, time.UTC)
	if got.Status != AvailabilityTimeParsed || got.CandidateAvailableAt == nil || !got.CandidateAvailableAt.Equal(want) {
		t.Fatalf("unexpected negative-offset boundary: got %+v, want %s", got, want)
	}
}

func TestParseDisclosureDateConservativeBoundaryRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		offset string
		reason string
	}{
		{name: "empty", raw: "", offset: "+08:00", reason: "news.available_at.disclosure_date.invalid_millis"},
		{name: "non numeric", raw: "1794931200000x", offset: "+08:00", reason: "news.available_at.disclosure_date.invalid_millis"},
		{name: "zero", raw: "0", offset: "+08:00", reason: "news.available_at.disclosure_date.invalid_millis"},
		{name: "negative", raw: "-1000", offset: "+08:00", reason: "news.available_at.disclosure_date.invalid_millis"},
		{name: "bad offset", raw: "1794931200000", offset: "08:00", reason: "news.available_at.disclosure_date.invalid_offset"},
		{name: "offset out of range", raw: "1794931200000", offset: "+25:00", reason: "news.available_at.disclosure_date.invalid_offset"},
		{name: "offset with seconds", raw: "1794931200000", offset: "+08:00:30", reason: "news.available_at.disclosure_date.invalid_offset"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := ParseDisclosureDateConservativeBoundary(test.raw, test.offset)
			if got.Status != AvailabilityTimeUnparseable || got.ReasonCode != test.reason ||
				got.CandidateAvailableAt != nil {
				t.Fatalf("unexpected result for %s: %+v", test.name, got)
			}
		})
	}
}

// 冻结的通用解析器必须继续拒绝纯日期值——保守边界只能通过专用入口产生。
func TestGeneralParserStillRejectsDateOnly(t *testing.T) {
	for _, raw := range []string{"1794931200000", "2026-11-18"} {
		got := ParseAvailabilityTime(raw)
		if got.CandidateAvailableAt != nil {
			t.Fatalf("general parser must not produce a candidate for %q: %+v", raw, got)
		}
	}
}
