package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

// IdentityCredential 是来源原生文章 ID；其唯一范围仅为 SourceID。
type IdentityCredential struct {
	SourceID   string
	Value      string
	Hash       string
	NewsID     string
	ObservedAt time.Time
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

// BuildIdentityCredentials 只构造来源原生文章 ID。规范 URL 由全局 URL 身份单独处理。
func BuildIdentityCredentials(sourceID, sourceNativeID string) ([]IdentityCredential, error) {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return nil, errors.New("news.identity.source_required")
	}
	sourceNativeID = strings.TrimSpace(sourceNativeID)
	if sourceNativeID == "" {
		return nil, nil
	}
	return []IdentityCredential{newIdentityCredential(sourceID, sourceNativeID)}, nil
}

func newIdentityCredential(sourceID, value string) IdentityCredential {
	return IdentityCredential{
		SourceID: sourceID,
		Value: value,
		Hash: identityValueHash(value),
	}
}

func identityValueHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

// ResolveIdentity 只在同一来源内解析原生文章 ID；Hash 命中后必须比较原文。
func ResolveIdentity(sourceID string, candidate, existing []IdentityCredential) IdentityResolution {
	sourceID = strings.TrimSpace(sourceID)
	if sourceID == "" {
		return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.source_required"}
	}
	matchedNewsIDs := make(map[string]struct{})
	for _, wanted := range candidate {
		if wanted.SourceID != sourceID || wanted.Value == "" ||
			wanted.Value != strings.TrimSpace(wanted.Value) ||
			wanted.Hash != identityValueHash(wanted.Value) {
			return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.candidate_invalid"}
		}
		for _, stored := range existing {
			if stored.SourceID != sourceID || stored.Hash != wanted.Hash {
				continue
			}
			if stored.Value != wanted.Value {
				return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.hash_collision"}
			}
			if stored.Hash != identityValueHash(stored.Value) {
				return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.stored_hash_mismatch"}
			}
			if stored.NewsID == "" {
				return IdentityResolution{Status: IdentityConflict, Reason: "news.identity.existing_owner_missing"}
			}
			matchedNewsIDs[stored.NewsID] = struct{}{}
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
