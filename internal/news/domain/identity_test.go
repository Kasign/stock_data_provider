package domain

import "testing"

func identityPolicy() SourceURLPolicy {
	return SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}}
}

func TestBuildIdentityCredentialsUsesNativeIDAndCanonicalURL(t *testing.T) {
	got, err := BuildIdentityCredentials("source-1", " item-7 ", "HTTPS://NEWS.EXAMPLE/story#top", identityPolicy())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("credentials=%+v", got)
	}
	if got[0].Type != IdentifierSourceNativeID || got[0].Value != "item-7" {
		t.Fatalf("native id=%+v", got[0])
	}
	if got[1].Type != IdentifierNormalizedURL || got[1].Value != "https://news.example/story" {
		t.Fatalf("url=%+v", got[1])
	}
	if got[0].Hash == "" || got[0].Hash != newIdentityCredential("source-1", IdentifierSourceNativeID, "item-7").Hash {
		t.Fatalf("hash=%q", got[0].Hash)
	}
}

func TestBuildIdentityCredentialsRequiresSourceAndAtLeastOneCredential(t *testing.T) {
	if _, err := BuildIdentityCredentials("", "id", "", identityPolicy()); err == nil {
		t.Fatal("expected source required")
	}
	if _, err := BuildIdentityCredentials("source-1", "", "", identityPolicy()); err == nil {
		t.Fatal("expected credential required")
	}
	if _, err := BuildIdentityCredentials("source-1", "", "https://evil.example/x", identityPolicy()); err == nil {
		t.Fatal("expected URL allowlist rejection")
	}
}

func TestResolveIdentityMatchesOnlyWithinSameSourceAndComparesHashValue(t *testing.T) {
	candidate := newIdentityCredential("source-1", IdentifierSourceNativeID, "same-id")
	otherSource := newIdentityCredential("source-2", IdentifierSourceNativeID, "same-id")
	if got := ResolveIdentity("source-1", []IdentityCredential{candidate}, []IdentityCredential{{SourceID: otherSource.SourceID, Type: otherSource.Type, Value: otherSource.Value, Hash: otherSource.Hash, NewsID: "news-2", Status: IdentifierActive}}); got.Status != IdentityNew {
		t.Fatalf("cross-source match=%+v", got)
	}

	collision := candidate
	collision.Value = "different-original-value"
	if got := ResolveIdentity("source-1", []IdentityCredential{candidate}, []IdentityCredential{{SourceID: "source-1", Type: candidate.Type, Value: collision.Value, Hash: candidate.Hash, NewsID: "news-1", Status: IdentifierActive}}); got.Status != IdentityConflict || got.Reason != "news.identity.hash_collision" {
		t.Fatalf("collision=%+v", got)
	}
}

func TestResolveIdentityDetectsNativeIDAndURLPointingToDifferentItems(t *testing.T) {
	nativeID := newIdentityCredential("source-1", IdentifierSourceNativeID, "item-1")
	urlID := newIdentityCredential("source-1", IdentifierNormalizedURL, "https://news.example/item")
	candidate := []IdentityCredential{nativeID, urlID}
	existing := []IdentityCredential{
		{SourceID: nativeID.SourceID, Type: nativeID.Type, Value: nativeID.Value, Hash: nativeID.Hash, Status: IdentifierActive, NewsID: "news-1"},
		{SourceID: urlID.SourceID, Type: urlID.Type, Value: urlID.Value, Hash: urlID.Hash, Status: IdentifierHistorical, NewsID: "news-2"},
	}
	got := ResolveIdentity("source-1", candidate, existing)
	if got.Status != IdentityConflict || got.Reason != "news.identity.multiple_items" {
		t.Fatalf("resolution=%+v", got)
	}
}

func TestResolveIdentityMatchesHistoricalURLAndIgnoresInvalidIdentifier(t *testing.T) {
	candidate := newIdentityCredential("source-1", IdentifierNormalizedURL, "https://news.example/old")
	existing := []IdentityCredential{
		{SourceID: candidate.SourceID, Type: candidate.Type, Value: candidate.Value, Hash: candidate.Hash, Status: IdentifierHistorical, NewsID: "news-1"},
		{SourceID: candidate.SourceID, Type: candidate.Type, Value: candidate.Value, Hash: candidate.Hash, Status: IdentifierInvalid, NewsID: "news-2"},
	}
	got := ResolveIdentity("source-1", []IdentityCredential{candidate}, existing)
	if got.Status != IdentityMatched || got.NewsID != "news-1" {
		t.Fatalf("resolution=%+v", got)
	}
}

func TestResolveIdentityRejectsCandidateFromDifferentSource(t *testing.T) {
	candidate := newIdentityCredential("source-2", IdentifierSourceNativeID, "id")
	got := ResolveIdentity("source-1", []IdentityCredential{candidate}, nil)
	if got.Status != IdentityConflict || got.Reason != "news.identity.candidate_invalid" {
		t.Fatalf("resolution=%+v", got)
	}
}
