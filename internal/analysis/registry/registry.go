package registry

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var (
	ErrFrozen      = errors.New("registry is frozen")
	ErrDuplicateID = errors.New("duplicate registry id")
	ErrInvalidID   = errors.New("registry item id is empty")
	ErrNotFound    = errors.New("registry item not found")
)

type Identified interface {
	ID() string
}

// Registry provides deterministic, duplicate-safe module registration.
// Register modules during application startup, validate them, then Freeze.
// Runtime code should not mutate a frozen registry.
type Registry[T Identified] struct {
	mu     sync.RWMutex
	items  map[string]T
	frozen bool
}

func New[T Identified]() *Registry[T] {
	return &Registry[T]{items: make(map[string]T)}
}

func (r *Registry[T]) Register(item T) error {
	if r == nil {
		return errors.New("nil registry")
	}
	id := strings.TrimSpace(item.ID())
	if id == "" {
		return ErrInvalidID
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.frozen {
		return ErrFrozen
	}
	if _, exists := r.items[id]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateID, id)
	}
	r.items[id] = item
	return nil
}

func (r *Registry[T]) Get(id string) (T, error) {
	var zero T
	if r == nil {
		return zero, errors.New("nil registry")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	if !ok {
		return zero, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return item, nil
}

func (r *Registry[T]) AllSorted() []T {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.items))
	for id := range r.items {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]T, 0, len(ids))
	for _, id := range ids {
		out = append(out, r.items[id])
	}
	return out
}

func (r *Registry[T]) Freeze() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frozen = true
}

func (r *Registry[T]) IsFrozen() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.frozen
}

// Len returns the number of registered modules.
func (r *Registry[T]) Len() int {
	if r == nil {
		return 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.items)
}
