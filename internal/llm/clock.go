package llm

import "time"

// Clock provides the current UTC time for deterministic validation and timing.
type Clock interface {
	Now() time.Time
}

// SystemClock is the production clock implementation.
type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}
