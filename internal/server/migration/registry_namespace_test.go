package migration

import (
	"testing"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestValidateAppliedNamespacesRequiresExactNamespaceSet(t *testing.T) {
	migration := func(version uint64) platformstorage.Migration {
		return platformstorage.Migration{Version: version}
	}
	expected := map[string][]platformstorage.Migration{
		"news":     []platformstorage.Migration{migration(1)},
		"research": []platformstorage.Migration{migration(1), migration(2)},
	}
	valid := map[string]map[uint64]string{
		"news":     map[uint64]string{1: "applied"},
		"research": map[uint64]string{1: "applied", 2: "applied"},
	}
	if err := validateAppliedNamespaces(expected, valid); err != nil {
		t.Fatalf("complete migration namespace set rejected: %v", err)
	}

	cases := []struct {
		name    string
		applied map[string]map[uint64]string
	}{
		{name: "unknown namespace", applied: map[string]map[uint64]string{
			"news":     map[uint64]string{1: "applied"},
			"research": map[uint64]string{1: "applied", 2: "applied"},
			"retired":  map[uint64]string{1: "applied"},
		}},
		{name: "missing namespace", applied: map[string]map[uint64]string{
			"news": map[uint64]string{1: "applied"},
		}},
		{name: "unknown version in registered namespace", applied: map[string]map[uint64]string{
			"news":     map[uint64]string{1: "applied", 2: "applied"},
			"research": map[uint64]string{1: "applied", 2: "applied"},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateAppliedNamespaces(expected, tc.applied); err == nil {
				t.Fatal("expected schema namespace validation error")
			}
		})
	}
}
