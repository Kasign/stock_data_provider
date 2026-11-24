package application

import (
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
)

type registryResolverTestRegistry struct {
	version string
	fingerprint string
}

func (registry registryResolverTestRegistry) Version() string { return registry.version }
func (registry registryResolverTestRegistry) Fingerprint() string { return registry.fingerprint }
func (registry registryResolverTestRegistry) Validate(stageoutput.Envelope, stageoutput.ValidationContext) error { return nil }
func (registry registryResolverTestRegistry) Entry(string, string, string) (stageoutput.Entry, bool) {
	return stageoutput.Entry{}, false
}

func TestStageOutputRegistrySetResolvesExactPersistedVersion(t *testing.T) {
	oldRegistry := registryResolverTestRegistry{version: "1.0.0", fingerprint: "old-fingerprint"}
	newRegistry := registryResolverTestRegistry{version: "1.1.0", fingerprint: "new-fingerprint"}
	set, err := NewStageOutputRegistrySet(oldRegistry, newRegistry)
	if err != nil {
		t.Fatalf("NewStageOutputRegistrySet() error = %v", err)
	}
	gotOld, err := set.Resolve("1.0.0")
	if err != nil || gotOld.Fingerprint() != oldRegistry.fingerprint {
		t.Fatalf("Resolve(1.0.0) = %v, %v", gotOld, err)
	}
	gotNew, err := set.Resolve("1.1.0")
	if err != nil || gotNew.Fingerprint() != newRegistry.fingerprint {
		t.Fatalf("Resolve(1.1.0) = %v, %v", gotNew, err)
	}
}

func TestStageOutputRegistrySetRejectsUnknownAndDuplicateVersions(t *testing.T) {
	first := registryResolverTestRegistry{version: "1.0.0", fingerprint: "first"}
	second := registryResolverTestRegistry{version: "1.0.0", fingerprint: "second"}
	set, err := NewStageOutputRegistrySet(first)
	if err != nil {
		t.Fatalf("NewStageOutputRegistrySet() error = %v", err)
	}
	if _, err := set.Resolve("2.0.0"); !errors.Is(err, ErrStageOutputRegistryNotFound) {
		t.Fatalf("Resolve(2.0.0) error = %v, want ErrStageOutputRegistryNotFound", err)
	}
	if _, err := NewStageOutputRegistrySet(first, second); !errors.Is(err, ErrInvalidStageOutputRegistrySet) {
		t.Fatalf("duplicate Registry version error = %v, want ErrInvalidStageOutputRegistrySet", err)
	}
	if _, err := NewStageOutputRegistrySet(nil); !errors.Is(err, ErrInvalidStageOutputRegistrySet) {
		t.Fatalf("nil Registry error = %v, want ErrInvalidStageOutputRegistrySet", err)
	}
}
