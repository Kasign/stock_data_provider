// Package storage defines persistence errors shared by transport and adapters.
package storage

import "errors"

var (
	ErrRunNotFound    = errors.New("analysis run not found")
	ErrRunNotFinished = errors.New("analysis run has no persisted result yet")
)
