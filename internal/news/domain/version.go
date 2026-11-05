package domain

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"hash"
	"strings"
	"time"
)

// BuildVersionFingerprint 生成新闻内容版本指纹。
// 仅纳入来源侧可形成版本的元数据与内容；Provider 摘要、采集时间和查询上下文不参与。
func BuildVersionFingerprint(candidate Candidate, content ContentLayers) string {
	digest := sha256.New()
	writeFingerprintField(digest, "news-version-v1")
	writeFingerprintField(digest, normalizeCandidateText(candidate.Title))
	writeFingerprintField(digest, strings.TrimSpace(candidate.PublisherName))
	writeFingerprintField(digest, strings.ToLower(strings.TrimSpace(candidate.PublisherDomain)))
	// 规范 URL 是新闻版本元数据；ProviderItemID 属于身份凭据，不参与内容指纹。
	writeFingerprintField(digest, normalizeCandidateURL(candidate.RawURL))
	writeFingerprintField(digest, publishedTimeFingerprint(candidate))
	writeFingerprintField(digest, normalizeCandidateText(content.SourceSummary))
	writeFingerprintField(digest, string(content.SourceSummaryOrigin))

	if content.Body == nil {
		writeFingerprintField(digest, "body:none")
	} else if body := NormalizeContentLayers(ContentLayers{Body: content.Body}).Body; body == nil {
		writeFingerprintField(digest, "body:none")
	} else {
		writeFingerprintField(digest, "body:present")
		writeFingerprintField(digest, string(body.Origin))
		writeFingerprintField(digest, string(body.Format))
		writeFingerprintField(digest, string(body.Kind))
		writeFingerprintField(digest, strings.ReplaceAll(body.Text, "\r\n", "\n"))
	}

	return hex.EncodeToString(digest.Sum(nil))
}

func normalizeCandidateURL(raw string) string {
	return strings.TrimSpace(raw)
}

func publishedTimeFingerprint(candidate Candidate) string {
	if candidate.PublishedAt != nil {
		return candidate.PublishedAt.UTC().Format(time.RFC3339Nano)
	}
	return strings.TrimSpace(candidate.PublishedAtRaw)
}

func writeFingerprintField(digest hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	_, _ = digest.Write(size[:])
	_, _ = digest.Write([]byte(value))
}
