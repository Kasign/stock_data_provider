package parser

import (
	"bytes"
	"errors"
	"fmt"
	"mime"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/charset"
)

var (
	ErrUnsupportedHTML = errors.New("news: unsupported html content type")
	ErrInvalidHTML     = errors.New("news: invalid html document")
)

// HTMLDocument 是离线解析结果；CanonicalURL 等页面声明值仍是不可信输入。
type HTMLDocument struct {
	Title          string
	Description    string
	Text           string
	CanonicalURL   string
	PublishedAtRaw string
	PublishedAt    *time.Time
	ObservedAt     time.Time
}

// ParseHTMLDocument 只解析已取得的字节，不执行网络请求、脚本或资源加载。
// 响应体大小限制、来源授权和 URL 安全校验必须由调用方/公共 HTTP 层负责。
func ParseHTMLDocument(data []byte, contentType string, observedAt time.Time) (HTMLDocument, error) {
	if len(bytes.TrimSpace(data)) == 0 || observedAt.IsZero() {
		return HTMLDocument{}, ErrInvalidHTML
	}
	if strings.TrimSpace(contentType) != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil {
			return HTMLDocument{}, ErrInvalidHTML
		}
		if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
			return HTMLDocument{}, ErrUnsupportedHTML
		}
	}

	reader, err := charset.NewReader(bytes.NewReader(data), contentType)
	if err != nil {
		return HTMLDocument{}, fmt.Errorf("%w: %v", ErrInvalidHTML, err)
	}
	root, err := html.Parse(reader)
	if err != nil {
		return HTMLDocument{}, fmt.Errorf("%w: %v", ErrInvalidHTML, err)
	}

	doc := HTMLDocument{ObservedAt: observedAt.UTC()}
	doc.Title = firstNonEmpty(metaContent(root, "property", "og:title"), nodeText(findFirst(root, "title")))
	doc.Description = firstNonEmpty(
		metaContent(root, "name", "description"),
		metaContent(root, "property", "og:description"),
	)
	doc.CanonicalURL = canonicalLink(root)
	doc.PublishedAtRaw = firstNonEmpty(
		metaContent(root, "property", "article:published_time"),
		metaContent(root, "name", "datePublished"),
		metaContent(root, "itemprop", "datePublished"),
	)
	if publishedAt, ok := parseFeedTime(doc.PublishedAtRaw); ok {
		publishedAt = publishedAt.UTC()
		doc.PublishedAt = &publishedAt
	}

	contentRoot := findFirst(root, "article")
	if contentRoot == nil {
		contentRoot = findFirst(root, "main")
	}
	if contentRoot == nil {
		contentRoot = findFirst(root, "body")
	}
	if contentRoot != nil {
		doc.Text = extractVisibleText(contentRoot)
	}
	return doc, nil
}

func metaContent(root *html.Node, key, value string) string {
	var result string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || result != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "meta" {
			for _, attr := range node.Attr {
				if strings.EqualFold(attr.Key, key) && strings.EqualFold(strings.TrimSpace(attr.Val), value) {
					for _, content := range node.Attr {
						if content.Key == "content" {
							result = strings.TrimSpace(content.Val)
							return
						}
					}
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return result
}

func canonicalLink(root *html.Node) string {
	var result string
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node == nil || result != "" {
			return
		}
		if node.Type == html.ElementNode && node.Data == "link" {
			var rel, href string
			for _, attr := range node.Attr {
				switch strings.ToLower(attr.Key) {
				case "rel":
					rel = strings.ToLower(attr.Val)
				case "href":
					href = strings.TrimSpace(attr.Val)
				}
			}
			for _, token := range strings.Fields(rel) {
				if token == "canonical" && href != "" {
					result = href
					return
				}
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return result
}

func findFirst(root *html.Node, tag string) *html.Node {
	if root == nil {
		return nil
	}
	if root.Type == html.ElementNode && root.Data == tag {
		return root
	}
	for child := root.FirstChild; child != nil; child = child.NextSibling {
		if found := findFirst(child, tag); found != nil {
			return found
		}
	}
	return nil
}

func nodeText(root *html.Node) string {
	if root == nil {
		return ""
	}
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
			builder.WriteByte(' ')
			return
		}
		if node.Type == html.ElementNode && isIgnoredHTMLNode(node.Data) {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return normalizeText(builder.String())
}

func extractVisibleText(root *html.Node) string {
	var builder strings.Builder
	var walk func(*html.Node)
	walk = func(node *html.Node) {
		if node.Type == html.TextNode {
			builder.WriteString(node.Data)
			builder.WriteByte(' ')
			return
		}
		if node.Type == html.ElementNode {
			if isIgnoredHTMLNode(node.Data) {
				return
			}
			if isBlockHTMLNode(node.Data) {
				builder.WriteByte('\n')
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if node.Type == html.ElementNode && isBlockHTMLNode(node.Data) {
			builder.WriteByte('\n')
		}
	}
	walk(root)
	return normalizeHTMLLines(builder.String())
}

func isIgnoredHTMLNode(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "template", "svg", "canvas",
		"iframe", "object", "embed", "nav", "header", "footer", "aside",
		"form", "button", "input", "select", "textarea":
		return true
	default:
		return false
	}
}

func isBlockHTMLNode(tag string) bool {
	switch tag {
	case "address", "article", "blockquote", "br", "dd", "div", "dl", "dt",
		"figcaption", "figure", "h1", "h2", "h3", "h4", "h5", "h6",
		"hr", "li", "main", "ol", "p", "pre", "section", "table", "tr", "ul":
		return true
	default:
		return false
	}
}

func normalizeHTMLLines(value string) string {
	lines := strings.Split(value, "\n")
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = normalizeText(line)
		if line != "" {
			normalized = append(normalized, line)
		}
	}
	return strings.Join(normalized, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

