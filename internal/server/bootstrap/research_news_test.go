package bootstrap

import (
	"database/sql"
	"testing"

	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

func TestNewResearchNewsCompositionRequiresConfiguredCursorKeyRing(t *testing.T) {
	cfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(func(string) (string, bool) {
		return "", false
	}))
	if err != nil {
		t.Fatalf("load empty News config: %v", err)
	}
	if _, err := newResearchNewsComposition(&sql.DB{}, cfg); err == nil {
		t.Fatal("composition without cursor key ring unexpectedly succeeded")
	}
}

func TestNewResearchNewsCompositionBuildsProductionAdapters(t *testing.T) {
	values := map[string]string{
		newsconfig.EvidenceCursorActiveKeyIDEnv: "active-v2",
		newsconfig.EvidenceCursorKeyRingEnv:     `{"active-v2":"0123456789abcdef0123456789abcdef","previous-v1":"abcdef0123456789abcdef0123456789"}`,
	}
	cfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, ok := values[key]
		return value, ok
	}))
	if err != nil {
		t.Fatalf("load News config: %v", err)
	}
	composition, err := newResearchNewsComposition(&sql.DB{}, cfg)
	if err != nil {
		t.Fatalf("newResearchNewsComposition() error = %v", err)
	}
	if composition == nil || composition.query == nil || composition.research == nil ||
		composition.importer == nil || composition.authorizer == nil {
		t.Fatal("composition did not construct every production dependency")
	}
}
