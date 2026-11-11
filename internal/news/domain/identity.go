package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
)

type IdentifierType string

const (
	IdentifierSourceNativeID IdentifierType = "source_native_id"
	IdentifierNormalizedURL  IdentifierType = "normalized_url"
)

type IdentifierStatus string

const (
	IdentifierActive     IdentifierStatus = "active"
	IdentifierHistorical IdentifierStatus = "historical"
	IdentifierInvalid    IdentifierStatus = "invalid"
)

type IdentityCredential struct {
	SourceID string
	Type     IdentifierType
	Value    string
	Hash     string
	Status   IdentifierStatus
	NewsID   string
}

type IdentityResolutionStatus string

const (
	IdentityNew      IdentityResolutionStatus = "new"
	IdentityMatched  IdentityResolutionStatus = "matched"
	IdentityConflict IdentityResolutionStatus = "conflict"
)

type IdentityResolution struct {
	Status IdentityResolutionStatus
	NewsID string
	Reason string
}

// BuildIdentityCredentials 只构造来源内身份凭据；URL 必须先通过来源 allowlist 校验。
func BuildIdentityCredentials(sourceID, sourceNativeID, rawURL string, policy SourceURLPolicy) ([]IdentityCredential, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return nil, errors.New("news.identity.source_required")
	}

	credentials := make([]IdentityCredential, 0, 2)
	if value := strings.TrimSpace(sourceNativeID); value != "" {
		credentials = append(credentials, newIdentityCredential(sourceID, IdentifierSourceNativeID, value))
	}
	if strings.TrimSpace(rawURL) != "" {
		decision := EvaluateSourceURL(rawURL, policy)
		if !decision.Allowed {
			// URL 不能作为身份凭据，但不应抹掉同一来源下已提供的原生条目 ID。
			if len(credentials) == 0 {
				return nil, errors.New("news.identity.url_rejected")
			}
		} else {
			credentials = append(credentials, newIdentityCredential(sourceID, IdentifierNormalizedURL, decision.CanonicalURL))
		}
	}
	if len(credentials) == 0 {
		return nil, errors.New("news.identity.credential_required")
	}
	return credentials, nil
}

func newIdentityCredential(sourceID string, kind IdentifierType, value string) IdentityCredential {
	return IdentityCredential{
		SourceID: sourceID,
		Type:     kind,
		Value:    value,
		Hash:     identityValueHash(value),
		Status:   IdentifierActive,
	}
}

func identityValueHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// ResolveIdentity 仅在同一来源内匹配凭据。Hash 命中后比较原文；任何多主体命中均隔离为冲突。
func ResolveIdentity(sourceID string, candidate, existing []IdentityCredential) IdentityResolution {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" || len(candidate) == 0 {
		return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.input_invalid"}
	}

	matchedNewsIDs := make(map[string]struct{})
	for _, wanted := range candidate {
		if wanted.SourceID != sourceID || wanted.Value == "" || wanted.Hash != identityValueHash(wanted.Value) ||
			(wanted.Type != IdentifierSourceNativeID && wanted.Type != IdentifierNormalizedURL) {
			return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.candidate_invalid"}
		}
		for _, stored := range existing {
			if stored.SourceID != sourceID || (stored.Status != IdentifierActive && stored.Status != IdentifierHistorical) {
				continue
			}
			if stored.Type != wanted.Type {
				continue
			}
			if stored.Value == wanted.Value {
				if stored.Hash != wanted.Hash {
					return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.stored_hash_mismatch"}
				}
				if stored.NewsID == "" {
					return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.existing_owner_missing"}
				}
				matchedNewsIDs[stored.NewsID] = struct{}{}
				continue
			}
			if stored.Hash == wanted.Hash {
				return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.hash_collision"}
			}
		}
	}

	switch len(matchedNewsIDs) {
	case 0:
		return IdentityResolution{Status: IdentityNew}
	case 1:
		for newsID := range matchedNewsIDs {
			return IdentityResolution{Status: IdentityMatched, NewsID: newsID}
		}
	default:
		return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.multiple_items"}
	}
	return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.unreachable"}
}
