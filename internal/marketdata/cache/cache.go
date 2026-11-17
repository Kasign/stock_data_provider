package cache

import (
	"context"
	"time"
)

// Availability reports whether a Cache implementation is an active external dependency.
// Implementations that do not expose this optional interface are treated as enabled.
type Availability interface{ Enabled() bool }

type Cache interface {
	Get(context.Context, string) ([]byte, bool, error)
	Set(context.Context, string, []byte, time.Duration) error
	Ping(context.Context) error
}

type disabled struct{}

func NewDisabled() Cache                                                  { return disabled{} }
func (disabled) Get(context.Context, string) ([]byte, bool, error)        { return nil, false, nil }
func (disabled) Set(context.Context, string, []byte, time.Duration) error { return nil }
func (disabled) Ping(context.Context) error                               { return nil }
func (disabled) Enabled() bool                                            { return false }
