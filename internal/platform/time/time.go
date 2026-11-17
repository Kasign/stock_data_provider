// Package time provides the shared time source and timezone operations used by
// application services. Persisted instants are UTC; local time is always an
// explicit presentation or business-calendar choice.
package time

import (
	"context"
	"errors"
	"fmt"
	stdtime "time"
)

// Source is the authoritative source of the current instant.
type Source interface {
	NowUTC(context.Context) (stdtime.Time, error)
}

// Clock exposes the process wall clock for code that needs a simple clock
// (for example, token expiry validation). It is not a cross-host authority.
type Clock interface {
	Now() stdtime.Time
}

// SystemClock reads the operating system wall clock.
type SystemClock struct{}

func (SystemClock) Now() stdtime.Time {
	return stdtime.Now().UTC()
}

// SystemSource adapts the operating system clock to Source.
type SystemSource struct {
	Clock Clock
}

func (s SystemSource) NowUTC(ctx context.Context) (stdtime.Time, error) {
	if ctx == nil {
		return stdtime.Time{}, errors.New("time context is required")
	}
	if err := ctx.Err(); err != nil {
		return stdtime.Time{}, err
	}
	clock := s.Clock
	if clock == nil {
		clock = SystemClock{}
	}
	return clock.Now().UTC(), nil
}

// Service centralizes time-source access and IANA timezone conversion.
type Service struct {
	source Source
}

func NewService(source Source) (*Service, error) {
	if source == nil {
		return nil, errors.New("time source is required")
	}
	return &Service{source: source}, nil
}

// NowUTC returns the current instant normalized to UTC.
func (s *Service) NowUTC(ctx context.Context) (stdtime.Time, error) {
	if s == nil || s.source == nil {
		return stdtime.Time{}, errors.New("time service is not configured")
	}
	now, err := s.source.NowUTC(ctx)
	if err != nil {
		return stdtime.Time{}, fmt.Errorf("read current time: %w", err)
	}
	if now.IsZero() {
		return stdtime.Time{}, errors.New("time source returned a zero instant")
	}
	return now.UTC(), nil
}

// NowIn returns the current instant represented in the requested IANA timezone.
func (s *Service) NowIn(ctx context.Context, timezone string) (stdtime.Time, error) {
	location, err := LoadLocation(timezone)
	if err != nil {
		return stdtime.Time{}, err
	}
	now, err := s.NowUTC(ctx)
	if err != nil {
		return stdtime.Time{}, err
	}
	return now.In(location), nil
}

// ToLocal converts an existing instant into the requested IANA timezone.
func ToLocal(instant stdtime.Time, timezone string) (stdtime.Time, error) {
	if instant.IsZero() {
		return stdtime.Time{}, errors.New("instant must not be zero")
	}
	location, err := LoadLocation(timezone)
	if err != nil {
		return stdtime.Time{}, err
	}
	return instant.In(location), nil
}

// LoadLocation validates and loads an IANA timezone identifier.
func LoadLocation(timezone string) (*stdtime.Location, error) {
	if timezone == "" {
		return nil, errors.New("IANA timezone is required")
	}
	location, err := stdtime.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load IANA timezone %q: %w", timezone, err)
	}
	return location, nil
}

// StartOfDay returns the beginning of the local calendar day containing instant.
func StartOfDay(instant stdtime.Time, timezone string) (stdtime.Time, error) {
	if instant.IsZero() {
		return stdtime.Time{}, errors.New("instant must not be zero")
	}
	location, err := LoadLocation(timezone)
	if err != nil {
		return stdtime.Time{}, err
	}
	local := instant.In(location)
	return stdtime.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, location), nil
}

// AddCalendarDays advances by local calendar days, respecting timezone rules
// such as daylight-saving transitions. It is not a fixed 24-hour duration.
func AddCalendarDays(instant stdtime.Time, days int, timezone string) (stdtime.Time, error) {
	if instant.IsZero() {
		return stdtime.Time{}, errors.New("instant must not be zero")
	}
	location, err := LoadLocation(timezone)
	if err != nil {
		return stdtime.Time{}, err
	}
	return instant.In(location).AddDate(0, 0, days), nil
}
