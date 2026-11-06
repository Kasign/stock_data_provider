package app

import (
	"errors"
	"strings"
	"testing"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestEvidenceCursorCodecRoundTripAndFilterCanonicalization(t *testing.T) {
	now := time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC)
	codec := newTestEvidenceCursorCodec(t, "key-a", map[string]string{
		"key-a": strings.Repeat("a", 32),
	})
	request := validEvidenceListRequest(now)
	request.Filters.NewsIDs = []string{"news-2", "news-1"}
	request.Filters.SourceRefs = []string{"source-2", "source-1"}
	last := ResearchEvidenceRecord{
		NewsID: "news-1", VersionID: "version-3",
		SystemObservedAt: now.Add(-2 * time.Minute),
		SystemAsOf:       request.SystemAsOf,
	}
	token, err := codec.EncodeNextCursor(request, "snapshot-1", last, now)
	if err != nil {
		t.Fatalf("EncodeNextCursor() error = %v", err)
	}
	reordered := request
	reordered.Filters = cloneEvidenceFilters(request.Filters)
	reordered.Filters.NewsIDs = []string{"news-1", "news-2"}
	reordered.Filters.SourceRefs = []string{"source-1", "source-2"}
	position, err := codec.DecodeCursor(token, reordered, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("DecodeCursor() error = %v", err)
	}
	if position.SnapshotRef != "snapshot-1" ||
		position.LastNewsID != "news-1" ||
		position.LastVersionID != "version-3" ||
		!position.LastSystemObservedAt.Equal(last.SystemObservedAt) ||
		!position.ExpiresAt.Equal(now.Add(evidenceCursorTTL)) {
		t.Fatalf("decoded cursor position = %+v", position)
	}
}

func TestEvidenceCursorCodecRejectsTamperingAndQueryMismatch(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	codec := newTestEvidenceCursorCodec(t, "key-a", map[string]string{
		"key-a": strings.Repeat("a", 32),
	})
	request := validEvidenceListRequest(now)
	last := ResearchEvidenceRecord{
		NewsID: "news-1", VersionID: "version-1",
		SystemObservedAt: request.SystemAsOf.Add(-time.Second),
		SystemAsOf:       request.SystemAsOf,
	}
	token, err := codec.EncodeNextCursor(request, "snapshot-1", last, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(token, ".")
	signature := parts[1]
	replacement := "A"
	if signature[:1] == replacement {
		replacement = "B"
	}
	tampered := parts[0] + "." + replacement + signature[1:]
	if _, err := codec.DecodeCursor(tampered, request, now.Add(time.Second)); !errors.Is(err, ErrEvidenceCursorInvalid) {
		t.Fatalf("tampered cursor error = %v", err)
	}
	changed := request
	changed.Filters = cloneEvidenceFilters(request.Filters)
	changed.Filters.NewsIDs = []string{"news-other"}
	if _, err := codec.DecodeCursor(token, changed, now.Add(time.Second)); !errors.Is(err, ErrEvidenceCursorInvalid) {
		t.Fatalf("filter mismatch error = %v", err)
	}
	changed = request
	changed.PageSize++
	if _, err := codec.DecodeCursor(token, changed, now.Add(time.Second)); !errors.Is(err, ErrEvidenceCursorInvalid) {
		t.Fatalf("page size mismatch error = %v", err)
	}
}

func TestEvidenceCursorCodecEnforcesExpiryAndIssueTime(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	codec := newTestEvidenceCursorCodec(t, "key-a", map[string]string{
		"key-a": strings.Repeat("a", 32),
	})
	request := validEvidenceListRequest(now)
	last := ResearchEvidenceRecord{
		NewsID: "news-1", VersionID: "version-1",
		SystemObservedAt: request.SystemAsOf.Add(-time.Second),
		SystemAsOf:       request.SystemAsOf,
	}
	token, err := codec.EncodeNextCursor(request, "snapshot-1", last, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codec.DecodeCursor(token, request, now.Add(evidenceCursorTTL)); !errors.Is(err, ErrEvidenceCursorExpired) {
		t.Fatalf("expired cursor error = %v", err)
	}
	if _, err := codec.DecodeCursor(token, request, now.Add(-time.Second)); !errors.Is(err, ErrEvidenceCursorInvalid) {
		t.Fatalf("future-issued cursor error = %v", err)
	}
}

func TestEvidenceCursorCodecSupportsKeyRotation(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	oldCodec := newTestEvidenceCursorCodec(t, "old", map[string]string{
		"old": strings.Repeat("o", 32),
	})
	request := validEvidenceListRequest(now)
	last := ResearchEvidenceRecord{
		NewsID: "news-1", VersionID: "version-1",
		SystemObservedAt: request.SystemAsOf.Add(-time.Second),
		SystemAsOf:       request.SystemAsOf,
	}
	oldToken, err := oldCodec.EncodeNextCursor(request, "snapshot-1", last, now)
	if err != nil {
		t.Fatal(err)
	}

	rotatedCodec := newTestEvidenceCursorCodec(t, "new", map[string]string{
		"old": strings.Repeat("o", 32),
		"new": strings.Repeat("n", 32),
	})
	if _, err := rotatedCodec.DecodeCursor(oldToken, request, now.Add(time.Second)); err != nil {
		t.Fatalf("old key should remain valid during TTL: %v", err)
	}
	newToken, err := rotatedCodec.EncodeNextCursor(request, "snapshot-2", last, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := oldCodec.DecodeCursor(newToken, request, now.Add(2*time.Second)); !errors.Is(err, ErrEvidenceCursorInvalid) {
		t.Fatalf("new key should not be accepted by old key ring: %v", err)
	}
}

func TestEvidenceCursorCodecRejectsWeakOrMissingKeys(t *testing.T) {
	weak := cursorSecret(strings.Repeat("x", 31))
	if _, err := NewEvidenceCursorCodec("weak", map[string]platformconfig.Secret{"weak": weak}); !errors.Is(err, ErrEvidenceCursorInvalid) {
		t.Fatalf("weak key error = %v", err)
	}
	strong := cursorSecret(strings.Repeat("x", 32))
	if _, err := NewEvidenceCursorCodec("missing", map[string]platformconfig.Secret{"other": strong}); !errors.Is(err, ErrEvidenceCursorInvalid) {
		t.Fatalf("missing active key error = %v", err)
	}
}

func newTestEvidenceCursorCodec(t *testing.T, active string, values map[string]string) *EvidenceCursorCodec {
	t.Helper()
	keys := make(map[string]platformconfig.Secret, len(values))
	for keyID, value := range values {
		keys[keyID] = cursorSecret(value)
	}
	codec, err := NewEvidenceCursorCodec(active, keys)
	if err != nil {
		t.Fatalf("NewEvidenceCursorCodec() error = %v", err)
	}
	return codec
}

func cursorSecret(value string) platformconfig.Secret {
	env := platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return value, true
	})
	return env.Secret("CURSOR_KEY")
}
