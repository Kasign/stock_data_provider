package parser

import (
	"bytes"
	"encoding/json"
	"mime"
	"strings"
	"time"
)

// FeedJSON 标记 official_api 的 JSON 响应文档，与 RSS/Atom 严格区分。
// 该格式只表达采集结果，不代表来源已授权或时间字段语义已通过审核。
const FeedJSON FeedFormat = "json"

// DetectOfficialAPIFormat 仅接受 application/json。独立于 DetectFeedFormat，
// 避免改变 RSS/Atom 采集路径对 application/json 的既有拒绝行为。
func DetectOfficialAPIFormat(contentType string) (FeedFormat, error) {
	if strings.TrimSpace(contentType) == "" {
		return "", ErrUnsupportedFeedContentType
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || strings.ToLower(mediaType) != "application/json" {
		return "", ErrUnsupportedFeedContentType
	}
	return FeedJSON, nil
}

// apiJSONDocument 只声明采集所需字段。未知字段被忽略，但不会被映射进 Candidate；
// announcementTime 用 json.RawMessage 保留原始字节，以便严格校验它必须是 JSON 整数
// （json.Number 会接受字符串字面量，无法区分类型）。
type apiJSONDocument struct {
	Announcements []apiJSONAnnouncement `json:"announcements"`
}

type apiJSONAnnouncement struct {
	AnnouncementID    string          `json:"announcementId"`
	AnnouncementTitle string          `json:"announcementTitle"`
	AnnouncementTime  json.RawMessage `json:"announcementTime"`
	AdjunctURL        string          `json:"adjunctUrl"`
}

// rawJSONInteger 返回严格 JSON 整数文本的原始字符串；接受负数、拒绝字符串/浮点/布尔/null。
func rawJSONInteger(raw json.RawMessage) (string, bool) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 {
		return "", false
	}
	cursor := 0
	if value[0] == '-' {
		cursor++
	}
	if cursor >= len(value) {
		return "", false
	}
	for i := cursor; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return "", false
		}
	}
	return string(value), true
}

// ParseAPIJSON 解析 official_api 的 JSON 响应为 FeedDocument。
// 关键边界：announcementTime 原样存入 PublishedAtRaw，PublishedAt 恒为 nil——
// 披露日编码不是精确首次公开时刻，不得在此处解析为时间（那是 DEC-073 资格链的职责）。
func ParseAPIJSON(data []byte, observedAt time.Time) (FeedDocument, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return FeedDocument{}, ErrInvalidFeed
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var doc apiJSONDocument
	if err := decoder.Decode(&doc); err != nil {
		return FeedDocument{}, ErrInvalidFeed
	}
	// 拒绝尾部多余内容，避免把拼接响应当作单一快照。
	var trailing interface{}
	if err := decoder.Decode(&trailing); err == nil {
		return FeedDocument{}, ErrInvalidFeed
	}
	result := FeedDocument{
		Format:     FeedJSON,
		ObservedAt: observedAt.UTC(),
		Entries:    make([]FeedEntry, 0, len(doc.Announcements)),
	}
	for _, announcement := range doc.Announcements {
		id := strings.TrimSpace(announcement.AnnouncementID)
		if id == "" {
			continue
		}
		rawTime, ok := rawJSONInteger(announcement.AnnouncementTime)
		if !ok {
			return FeedDocument{}, ErrInvalidFeed
		}
		entry := FeedEntry{
			ID:             id,
			Title:          strings.TrimSpace(announcement.AnnouncementTitle),
			Link:           strings.TrimSpace(announcement.AdjunctURL),
			PublishedAtRaw: rawTime,
			MissingFields:  make([]string, 0),
		}
		if entry.Title == "" {
			entry.MissingFields = append(entry.MissingFields, "announcementTitle")
		}
		if entry.Link == "" {
			entry.MissingFields = append(entry.MissingFields, "adjunctUrl")
		}
		result.Entries = append(result.Entries, entry)
	}
	return result, nil
}
