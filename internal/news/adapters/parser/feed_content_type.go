package parser

import (
	"errors"
	"mime"
	"strings"
)

var ErrUnsupportedFeedContentType = errors.New("news: unsupported feed content type")

// DetectFeedFormat 仅按 HTTP Content-Type 判断，不从 URL 后缀或响应正文猜测类型。
func DetectFeedFormat(contentType string) (FeedFormat, error) {
	if strings.TrimSpace(contentType) == "" {
		return "", ErrUnsupportedFeedContentType
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return "", ErrUnsupportedFeedContentType
	}
	switch strings.ToLower(mediaType) {
	case "application/rss+xml":
		return FeedRSS, nil
	case "application/atom+xml":
		return FeedAtom, nil
	default:
		return "", ErrUnsupportedFeedContentType
	}
}
