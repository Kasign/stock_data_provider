package storage

import (
	"database/sql"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func TestNewMySQLRepositoryRequiresBorrowedDatabase(t *testing.T) {
	if _, err := NewMySQLRepository(nil); err == nil {
		t.Fatal("NewMySQLRepository(nil) error = nil, want validation error")
	}
}

func TestIdentityAdvisoryLockNameIsBoundedAndDoesNotExposeSourceID(t *testing.T) {
	sourceID := "sensitive-source-id"
	name := identityAdvisoryLockName(sourceID)
	if len(name) > 64 {
		t.Fatalf("advisory lock name length = %d, exceeds MariaDB limit", len(name))
	}
	if strings.Contains(name, sourceID) {
		t.Fatal("advisory lock name exposes source ID")
	}
	if name != identityAdvisoryLockName(sourceID) {
		t.Fatal("advisory lock name is not deterministic")
	}
	if name == identityAdvisoryLockName("another-source") {
		t.Fatal("different source IDs share the same advisory lock name")
	}
}

func TestNullableStringUsesSQLNullForEmptyReferences(t *testing.T) {
	if nullableString("") != nil {
		t.Fatal("empty optional reference must map to SQL NULL")
	}
	if got := nullableString("version-1"); got != "version-1" {
		t.Fatalf("nullableString() = %#v", got)
	}
}

func TestEvidenceSnapshotPayloadSizeIsBounded(t *testing.T) {
	if validEvidenceSnapshotPayloadSize(nil) {
		t.Fatal("nil payload must be rejected")
	}
	if validEvidenceSnapshotPayloadSize([]byte{}) {
		t.Fatal("empty payload must be rejected")
	}
	if !validEvidenceSnapshotPayloadSize([]byte("snapshot")) {
		t.Fatal("non-empty payload within limit must be accepted")
	}
	if !validEvidenceSnapshotPayloadSize(make([]byte, maximumEvidenceSnapshotBytes)) {
		t.Fatal("payload at maximum size must be accepted")
	}
	if validEvidenceSnapshotPayloadSize(make([]byte, maximumEvidenceSnapshotBytes+1)) {
		t.Fatal("payload exceeding maximum size must be rejected")
	}
}

func TestAuthorizationSnapshotEnvelopeMatchesEveryIndexedField(t *testing.T) {
	binding := loadIntegrationAuthorizationBinding(t)
	ref := binding.AuthorizationRef
	decidedAt, err := time.Parse(time.RFC3339Nano, ref.DecidedAt)
	if err != nil {
		t.Fatal(err)
	}
	var expiresAt sql.NullTime
	if ref.ExpiresAt.Value != nil {
		parsed, err := time.Parse(time.RFC3339Nano, *ref.ExpiresAt.Value)
		if err != nil {
			t.Fatal(err)
		}
		expiresAt = sql.NullTime{Time: parsed, Valid: true}
	}
	matches := func(candidate domain.NewsAuthorizationBinding, authorizationRef, subjectRef, resourceRef, decision, purposeCode, contentLevel string, decided time.Time, expires sql.NullTime, fingerprint string) bool {
		return authorizationSnapshotEnvelopeMatches(candidate, authorizationRef, subjectRef, resourceRef,
			decision, purposeCode, contentLevel, decided, expires, fingerprint)
	}
	valid := func() bool {
		return matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(ref.Decision),
			ref.PurposeCode, string(ref.ContentLevel), decidedAt, expiresAt, ref.DecisionFingerprint)
	}
	if !valid() {
		t.Fatal("valid authorization envelope did not match")
	}
	changedContentLevel := domain.AuthorizationContentMetadata
	if ref.ContentLevel == changedContentLevel {
		changedContentLevel = domain.AuthorizationContentSummary
	}
	tests := []struct {
		name string
		mutate func() bool
	}{
		{name: "authorization ref", mutate: func() bool { return matches(binding, ref.AuthorizationRef+"-tampered", ref.SubjectRef, ref.ResourceRef, string(ref.Decision), ref.PurposeCode, string(ref.ContentLevel), decidedAt, expiresAt, ref.DecisionFingerprint) }},
		{name: "subject ref", mutate: func() bool { return matches(binding, ref.AuthorizationRef, ref.SubjectRef+"-tampered", ref.ResourceRef, string(ref.Decision), ref.PurposeCode, string(ref.ContentLevel), decidedAt, expiresAt, ref.DecisionFingerprint) }},
		{name: "resource ref", mutate: func() bool { return matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef+"-tampered", string(ref.Decision), ref.PurposeCode, string(ref.ContentLevel), decidedAt, expiresAt, ref.DecisionFingerprint) }},
		{name: "decision", mutate: func() bool { return matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(domain.AuthorizationDecisionDenied), ref.PurposeCode, string(ref.ContentLevel), decidedAt, expiresAt, ref.DecisionFingerprint) }},
		{name: "purpose", mutate: func() bool { return matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(ref.Decision), ref.PurposeCode+"-tampered", string(ref.ContentLevel), decidedAt, expiresAt, ref.DecisionFingerprint) }},
		{name: "content level", mutate: func() bool { return matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(ref.Decision), ref.PurposeCode, string(changedContentLevel), decidedAt, expiresAt, ref.DecisionFingerprint) }},
		{name: "decided at", mutate: func() bool { return matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(ref.Decision), ref.PurposeCode, string(ref.ContentLevel), decidedAt.Add(time.Microsecond), expiresAt, ref.DecisionFingerprint) }},
		{name: "expires at", mutate: func() bool { changed := expiresAt; if changed.Valid { changed.Time = changed.Time.Add(time.Microsecond) } else { changed = sql.NullTime{Time: decidedAt.Add(time.Hour), Valid: true} }; return matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(ref.Decision), ref.PurposeCode, string(ref.ContentLevel), decidedAt, changed, ref.DecisionFingerprint) }},
		{name: "fingerprint", mutate: func() bool { return matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(ref.Decision), ref.PurposeCode, string(ref.ContentLevel), decidedAt, expiresAt, strings.Repeat("f", 64)) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.mutate() {
				t.Fatal("tampered envelope unexpectedly matched")
			}
		})
	}
	// MariaDB stores DATETIME(6); sub-microsecond differences are intentionally
	// normalized because they cannot be represented by the persisted envelope.
	if !matches(binding, ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, string(ref.Decision),
		ref.PurposeCode, string(ref.ContentLevel), decidedAt.Truncate(time.Microsecond).Add(100*time.Nanosecond), expiresAt, ref.DecisionFingerprint) {
		t.Fatal("sub-microsecond timestamp difference should match DATETIME(6) precision")
	}
}
