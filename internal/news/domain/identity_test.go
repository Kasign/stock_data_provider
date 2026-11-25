package domain

import "testing"

func TestBuildIdentityCredentialsCreatesOnlySourceNativeID(t *testing.T) {
	got, err := BuildIdentityCredentials(" source-1 ", " item-7 ")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SourceID != "source-1" || got[0].Value != "item-7" || got[0].Hash == "" {
		t.Fatalf("credentials=%+v", got)
	}
}

func TestBuildIdentityCredentialsAllowsURLOnlyIdentityPath(t *testing.T) {
	got, err := BuildIdentityCredentials("source-1", "")
	if err != nil || len(got) != 0 {
		t.Fatalf("credentials=%+v err=%v", got, err)
	}
}

func TestBuildIdentityCredentialsRequiresSource(t *testing.T) {
	if _, err := BuildIdentityCredentials("", "id"); err == nil {
		t.Fatal("expected source required")
	}
}

func TestResolveIdentityMatchesOnlyWithinSource(t *testing.T) {
	candidate := newIdentityCredential("source-1", "same-id")
	otherSource := newIdentityCredential("source-2", "same-id")
	got := ResolveIdentity("source-1", []IdentityCredential{candidate}, []IdentityCredential{{
		SourceID: otherSource.SourceID, Value: otherSource.Value, Hash: otherSource.Hash, NewsID: "news-2",
	}})
	if got.Status != IdentityNew {
		t.Fatalf("cross-source match=%+v", got)
	}
	got = ResolveIdentity("source-1", []IdentityCredential{candidate}, []IdentityCredential{{
		SourceID: "source-1", Value: candidate.Value, Hash: candidate.Hash, NewsID: "news-1",
	}})
	if got.Status != IdentityMatched || got.NewsID != "news-1" {
		t.Fatalf("resolution=%+v", got)
	}
}

func TestResolveIdentityDetectsHashCollision(t *testing.T) {
	candidate := newIdentityCredential("source-1", "same-id")
	got := ResolveIdentity("source-1", []IdentityCredential{candidate}, []IdentityCredential{{
		SourceID: "source-1", Value: "different-value", Hash: candidate.Hash, NewsID: "news-1",
	}})
	if got.Status != IdentityConflict || got.Reason != "news.identity.hash_collision" {
		t.Fatalf("resolution=%+v", got)
	}
}

func TestResolveIdentityDetectsMultipleExistingOwners(t *testing.T) {
	first := newIdentityCredential("source-1", "first")
	second := newIdentityCredential("source-1", "second")
	got := ResolveIdentity("source-1", []IdentityCredential{first, second}, []IdentityCredential{
		{SourceID: first.SourceID, Value: first.Value, Hash: first.Hash, NewsID: "news-1"},
		{SourceID: second.SourceID, Value: second.Value, Hash: second.Hash, NewsID: "news-2"},
	})
	if got.Status != IdentityConflict || got.Reason != "news.identity.multiple_items" {
		t.Fatalf("resolution=%+v", got)
	}
}

func TestResolveIdentityRejectsTamperedCandidateHash(t *testing.T) {
	candidate := newIdentityCredential("source-1", "id")
	candidate.Hash = "bad"
	got := ResolveIdentity("source-1", []IdentityCredential{candidate}, nil)
	if got.Status != IdentityConflict || got.Reason != "news.identity.candidate_invalid" {
		t.Fatalf("resolution=%+v", got)
	}
}
