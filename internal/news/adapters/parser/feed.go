package parser

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"mime"
	"strconv"
	"strings"
	"time"
)

var (
	ErrUnsupportedFeed = errors.New("news: unsupported feed format")
	ErrInvalidFeed     = errors.New("news: invalid feed document")
)

type FeedFormat string

const (
	FeedRSS  FeedFormat = "rss"
	FeedAtom FeedFormat = "atom"
)

// FeedDocument 是离线解析后的 Feed 快照，不代表来源已获授权或内容可留存。
type FeedDocument struct {
	Format     FeedFormat
	Title      string
	Link       string
	ObservedAt time.Time
	Entries    []FeedEntry
}

// FeedEntry 保留来源原始身份和发布时间文本，不推断未知时区。
type FeedEntry struct {
	ID             string
	Title          string
	Link           string
	Summary        string
	Content        string
	Attachments    []FeedAttachment
	Author         string
	PublishedAtRaw string
	PublishedAt    *time.Time
	UpdatedAtRaw   string
	UpdatedAt      *time.Time
	MissingFields  []string
}

type rssDocument struct {
	XMLName xml.Name `xml:"rss"`
	Channel *struct {
		Title string    `xml:"title"`
		Link  string    `xml:"link"`
		Items []rssItem `xml:"item"`
	} `xml:"channel"`
}

type rssItem struct {
	GUID        string `xml:"guid"`
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Content     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Author      string `xml:"author"`
	Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	PubDate     string         `xml:"pubDate"`
	Enclosures  []rssEnclosure `xml:"enclosure"`
}

type rssEnclosure struct {
	URL    string `xml:"url,attr"`
	Type   string `xml:"type,attr"`
	Length string `xml:"length,attr"`
}

type atomDocument struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href   string `xml:"href,attr"`
	Rel    string `xml:"rel,attr"`
	Type   string `xml:"type,attr"`
	Length string `xml:"length,attr"`
	Title  string `xml:"title,attr"`
}

// FeedAttachment 仅保存 Feed 声明的附件元信息，不下载或信任附件 URL。
type FeedAttachment struct {
	URL        string
	MediaType  string
	RawLength  string
	Length     *int64
	Title      string
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Links     []atomLink `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
	Authors   []struct {
		Name string `xml:"name"`
	} `xml:"author"`
}

// parseRootName 同时验证 XML 只有一个根元素，避免 xml.Unmarshal 忽略尾随的第二份文档。
func parseRootName(data []byte) (xml.Name, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root struct {
		XMLName xml.Name
	}
	if err := decoder.Decode(&root); err != nil {
		return xml.Name{}, ErrInvalidFeed
	}
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return xml.Name{}, ErrInvalidFeed
		}
		switch value := token.(type) {
		case xml.StartElement:
			return xml.Name{}, ErrInvalidFeed
		case xml.CharData:
			if len(bytes.TrimSpace(value)) > 0 {
				return xml.Name{}, ErrInvalidFeed
			}
		}
	}
	return root.XMLName, nil
}

// ParseFeed 解析 RSS 2.0 或 Atom 1.0。函数不访问网络、不抓取 HTML 正文，也不判断授权。
func ParseFeed(data []byte, observedAt time.Time) (FeedDocument, error) {
	if observedAt.IsZero() || len(bytes.TrimSpace(data)) == 0 {
		return FeedDocument{}, ErrInvalidFeed
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	rootName, err := parseRootName(data)
	if err != nil {
		return FeedDocument{}, ErrInvalidFeed
	}

	switch rootName.Local {
	case "rss":
		var parsed rssDocument
		if err := xml.Unmarshal(data, &parsed); err != nil || parsed.Channel == nil {
			return FeedDocument{}, ErrInvalidFeed
		}
		doc := FeedDocument{
			Format: FeedRSS, Title: normalizeText(parsed.Channel.Title),
			Link: strings.TrimSpace(parsed.Channel.Link), ObservedAt: observedAt.UTC(),
			Entries: make([]FeedEntry, 0, len(parsed.Channel.Items)),
		}
		for _, item := range parsed.Channel.Items {
			author := item.Author
			if strings.TrimSpace(author) == "" {
				author = item.Creator
			}
			content := item.Content
			entry := newFeedEntry(item.GUID, item.Title, item.Link, item.Description, content, author, item.PubDate, "")
			for _, enclosure := range item.Enclosures {
				entry.Attachments = append(entry.Attachments, newFeedAttachment(enclosure.URL, enclosure.Type, enclosure.Length, ""))
			}
			doc.Entries = append(doc.Entries, entry)
		}
		return doc, nil
	case "feed":
		var parsed atomDocument
		if err := xml.Unmarshal(data, &parsed); err != nil {
			return FeedDocument{}, ErrInvalidFeed
		}
		doc := FeedDocument{
			Format: FeedAtom, Title: normalizeText(parsed.Title),
			Link: selectAtomLink(parsed.Links), ObservedAt: observedAt.UTC(),
			Entries: make([]FeedEntry, 0, len(parsed.Entries)),
		}
		for _, item := range parsed.Entries {
			author := ""
			if len(item.Authors) > 0 {
				author = item.Authors[0].Name
			}
			entry := newFeedEntry(item.ID, item.Title, selectAtomLink(item.Links), item.Summary, item.Content, author, item.Published, item.Updated)
			for _, link := range item.Links {
				if strings.EqualFold(strings.TrimSpace(link.Rel), "enclosure") {
					entry.Attachments = append(entry.Attachments, newFeedAttachment(link.Href, link.Type, link.Length, link.Title))
				}
			}
			doc.Entries = append(doc.Entries, entry)
		}
		return doc, nil
	default:
		return FeedDocument{}, ErrUnsupportedFeed
	}
}

func newFeedAttachment(rawURL, rawType, rawLength, title string) FeedAttachment {
	attachment := FeedAttachment{URL: strings.TrimSpace(rawURL), RawLength: strings.TrimSpace(rawLength), Title: normalizeText(title)}
	if mediaType, _, err := mime.ParseMediaType(strings.TrimSpace(rawType)); err == nil {
		attachment.MediaType = strings.ToLower(mediaType)
	}
	if length, err := strconv.ParseInt(attachment.RawLength, 10, 64); err == nil && length >= 0 {
		attachment.Length = &length
	}
	return attachment
}

func newFeedEntry(id, title, link, summary, content, author, publishedRaw, updatedRaw string) FeedEntry {
	entry := FeedEntry{
		ID: normalizeText(id), Title: normalizeText(title), Link: strings.TrimSpace(link),
		Summary: normalizeText(summary), Content: strings.TrimSpace(content),
		Author: normalizeText(author), PublishedAtRaw: strings.TrimSpace(publishedRaw), UpdatedAtRaw: strings.TrimSpace(updatedRaw),
	}
	if parsed, ok := parseFeedTime(entry.PublishedAtRaw); ok {
		parsed = parsed.UTC()
		entry.PublishedAt = &parsed
	}
	if parsed, ok := parseFeedTime(entry.UpdatedAtRaw); ok {
		parsed = parsed.UTC()
		entry.UpdatedAt = &parsed
	}
	if entry.ID == "" {
		entry.MissingFields = append(entry.MissingFields, "id")
	}
	if entry.Title == "" {
		entry.MissingFields = append(entry.MissingFields, "title")
	}
	if entry.Link == "" {
		entry.MissingFields = append(entry.MissingFields, "link")
	}
	if entry.PublishedAtRaw == "" || entry.PublishedAt == nil {
		entry.MissingFields = append(entry.MissingFields, "published_at")
	}
	if entry.Summary == "" && entry.Content == "" {
		entry.MissingFields = append(entry.MissingFields, "content")
	}
	return entry
}

func selectAtomLink(links []atomLink) string {
	for _, link := range links {
		if strings.TrimSpace(link.Href) != "" && (link.Rel == "" || link.Rel == "alternate") {
			return strings.TrimSpace(link.Href)
		}
	}
	return ""
}

func parseFeedTime(raw string) (time.Time, bool) {
	if raw == "" {
		return time.Time{}, false
	}
	layouts := []string{
		time.RFC3339Nano,
		time.RFC1123Z,
		time.RFC822Z,
	}
	for _, layout := range layouts {
		parsed, err := time.Parse(layout, raw)
		if err == nil && !parsed.IsZero() {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func normalizeText(value string) string {
	return strings.Join(strings.Fields(value), " ")
}
