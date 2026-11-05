package domain

import (
	"strings"
	"time"
)

// NormalizeCandidate 统一搜索候选的文本、时间和缺失字段语义。
// 不从 URL 推断发布者身份，也不把无时区日期解释为精确时间。
func NormalizeCandidate(candidate Candidate, observedAt time.Time, queryDimension string) Candidate {
	candidate.ProviderCode = strings.TrimSpace(candidate.ProviderCode)
	candidate.ProviderItemID = strings.TrimSpace(candidate.ProviderItemID)
	candidate.SourceItemID = strings.TrimSpace(candidate.SourceItemID)
	candidate.PublisherName = strings.TrimSpace(candidate.PublisherName)
	candidate.Author = normalizeCandidateText(candidate.Author)
	candidate.PublisherDomain = strings.ToLower(strings.TrimSpace(candidate.PublisherDomain))
	candidate.Title = normalizeCandidateText(candidate.Title)
	candidate.ProviderSummary = normalizeCandidateText(candidate.ProviderSummary)
	candidate.RawURL = strings.TrimSpace(candidate.RawURL)
	candidate.CanonicalURL = strings.TrimSpace(candidate.CanonicalURL)
	candidate.PublishedAtRaw = strings.TrimSpace(candidate.PublishedAtRaw)
	candidate.QueryDimension = strings.TrimSpace(candidate.QueryDimension)

	if candidate.ObservedAt.IsZero() {
		candidate.ObservedAt = observedAt
	}
	if !candidate.ObservedAt.IsZero() {
		candidate.ObservedAt = candidate.ObservedAt.UTC()
	}
	if candidate.QueryDimension == "" {
		candidate.QueryDimension = strings.TrimSpace(queryDimension)
	}
	if candidate.PublishedAt != nil {
		publishedAt := candidate.PublishedAt.UTC()
		candidate.PublishedAt = &publishedAt
	} else if parsed, ok := parseCandidateTimestamp(candidate.PublishedAtRaw); ok {
		parsed = parsed.UTC()
		candidate.PublishedAt = &parsed
	}

	candidate.MissingFields = candidate.MissingFields[:0]
	if candidate.Title == "" {
		candidate.MissingFields = append(candidate.MissingFields, "title")
	}
	if candidate.RawURL == "" {
		candidate.MissingFields = append(candidate.MissingFields, "url")
	}
	if candidate.PublishedAt == nil {
		candidate.MissingFields = append(candidate.MissingFields, "published_at")
	}
	if candidate.PublisherName == "" {
		candidate.MissingFields = append(candidate.MissingFields, "publisher")
	}
	return candidate
}

func parseCandidateTimestamp(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC1123Z, time.RFC822Z} {
		parsed, err := time.Parse(layout, raw)
		if err == nil && !parsed.IsZero() {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func normalizeCandidateText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
