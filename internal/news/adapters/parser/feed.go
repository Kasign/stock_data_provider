package parser

import (
	"bytes"
	"encoding/xml"
	"errors"
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
	Author         string
	PublishedAtRaw string
	PublishedAt    *time.Time
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
	PubDate     string `xml:"pubDate"`
}

type atomDocument struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
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

// ParseFeed 解析 RSS 2.0 或 Atom 1.0。函数不访问网络、不抓取 HTML 正文，也不判断授权。
func ParseFeed(data []byte, observedAt time.Time) (FeedDocument, error) {
	if observedAt.IsZero() || len(bytes.TrimSpace(data)) == 0 {
		return FeedDocument{}, ErrInvalidFeed
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})

	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(data, &root); err != nil {
		return FeedDocument{}, ErrInvalidFeed
	}

	switch root.XMLName.Local {
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
			doc.Entries = append(doc.Entries, newFeedEntry(
				item.GUID, item.Title, item.Link, item.Description, content, author, item.PubDate,
			))
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
			published := item.Published
			if strings.TrimSpace(published) == "" {
				published = item.Updated
			}
			author := ""
			if len(item.Authors) > 0 {
				author = item.Authors[0].Name
			}
			doc.Entries = append(doc.Entries, newFeedEntry(
				item.ID, item.Title, selectAtomLink(item.Links), item.Summary, item.Content, author, published,
			))
		}
		return doc, nil
	default:
		return FeedDocument{}, ErrUnsupportedFeed
	}
}

func newFeedEntry(id, title, link, summary, content, author, publishedRaw string) FeedEntry {
	entry := FeedEntry{
		ID: normalizeText(id), Title: normalizeText(title), Link: strings.TrimSpace(link),
		Summary: normalizeText(summary), Content: strings.TrimSpace(content),
		Author: normalizeText(author), PublishedAtRaw: strings.TrimSpace(publishedRaw),
	}
	if parsed, ok := parseFeedTime(entry.PublishedAtRaw); ok {
		parsed = parsed.UTC()
		entry.PublishedAt = &parsed
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
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		time.RFC850,
		time.ANSIC,
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
