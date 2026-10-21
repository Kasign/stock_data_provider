package registry

import (
	"errors"
	"testing"
)

type item string

func (i item) ID() string { return string(i) }

func TestRegistryDuplicateFreezeAndOrdering(t *testing.T) {
	r := New[item]()
	if err := r.Register(item("zeta")); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(item("alpha")); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(item("alpha")); !errors.Is(err, ErrDuplicateID) {
		t.Fatalf("want duplicate error, got %v", err)
	}
	items := r.AllSorted()
	if len(items) != 2 || items[0].ID() != "alpha" || items[1].ID() != "zeta" {
		t.Fatalf("unexpected order: %#v", items)
	}
	r.Freeze()
	if err := r.Register(item("later")); !errors.Is(err, ErrFrozen) {
		t.Fatalf("want frozen error, got %v", err)
	}
}
