package diagnostics

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
	platformdiag "example.com/stock-analysis-flow/internal/platform/diagnostics"
)

// BusinessQualityCheck evaluates one News evidence snapshot. Platform diagnostic
// status reports whether the assessment ran; business quality remains a separate
// News-owned attribute and must never be collapsed into Platform status.
func BusinessQualityCheck(
	evidenceID string,
	candidate domain.Candidate,
	content domain.ContentLayers,
	sourceState domain.SourceIdentityState,
) (platformdiag.Check, error) {
	evidenceID = strings.TrimSpace(evidenceID)
	if evidenceID == "" {
		return platformdiag.Check{}, errors.New("news diagnostics: evidence reference is required")
	}
	if candidate.PublishedAt != nil {
		publishedAt := *candidate.PublishedAt
		candidate.PublishedAt = &publishedAt
	}
	candidate.MissingFields = append([]string(nil), candidate.MissingFields...)
	content = domain.NormalizeContentLayers(content)

	sum := sha256.Sum256([]byte(evidenceID))
	checkID := "news.quality.evidence." + hex.EncodeToString(sum[:12])

	return platformdiag.Check{
		ID: checkID, Name: "News business quality assessment", Required: false, Timeout: time.Second,
		Run: func(ctx context.Context) (platformdiag.Observation, error) {
			if err := ctx.Err(); err != nil {
				return platformdiag.Observation{}, err
			}
			assessment := domain.AssessBusinessQuality(candidate, content, sourceState)
			attributes := map[string]string{
				"business_quality": string(assessment.Status),
				"metadata_quality": "unknown",
				"content_quality":  "unknown",
				"source_quality":   "unknown",
				"issue_count":      "0",
			}
			for _, dimension := range assessment.Dimensions {
				switch dimension.Dimension {
				case domain.QualityDimensionMetadata:
					attributes["metadata_quality"] = string(dimension.Status)
				case domain.QualityDimensionContent:
					attributes["content_quality"] = string(dimension.Status)
				case domain.QualityDimensionSource:
					attributes["source_quality"] = string(dimension.Status)
				}
			}
			issueCodes := make([]string, 0, len(assessment.Issues))
			for _, issue := range assessment.Issues {
				if validQualityIssueCode(issue.Code) && len(issueCodes) < 8 {
					issueCodes = append(issueCodes, issue.Code)
				}
			}
			attributes["issue_count"] = strconv.Itoa(len(assessment.Issues))
			if len(issueCodes) > 0 {
				attributes["issue_codes"] = strings.Join(issueCodes, ",")
			}
			return platformdiag.Observation{
				Status: platformdiag.StatusHealthy,
				SafeMessage: "News business quality assessment completed",
				CheckedAt: time.Now().UTC(),
				Attributes: attributes,
			}, nil
		},
	}, nil
}

func validQualityIssueCode(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '_' {
			continue
		}
		return false
	}
	return true
}

