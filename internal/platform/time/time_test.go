package time

import (
	"context"
	"errors"
	"testing"
	stdtime "time"
)

type fixedSource struct {
	value stdtime.Time
	err   error
}

func (s fixedSource) NowUTC(context.Context) (stdtime.Time, error) {
	return s.value, s.err
}

func TestServiceNowUTCNormalizesSource(t *testing.T) {
	value := stdtime.Date(2026, 11, 17, 8, 0, 0, 0, stdtime.FixedZone("offset", 8*60*60))
	service, err := NewService(fixedSource{value: value})
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.NowUTC(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Location() != stdtime.UTC || !got.Equal(value) {
		t.Fatalf("NowUTC() = %s (%s), want same instant in UTC", got, got.Location())
	}
}

func TestServiceNowInUsesIANAZone(t *testing.T) {
	value := stdtime.Date(2026, 11, 17, 8, 0, 0, 0, stdtime.UTC)
	service, err := NewService(fixedSource{value: value})
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.NowIn(context.Background(), "Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hour() != 16 || got.Location().String() != "Asia/Singapore" {
		t.Fatalf("NowIn() = %s, want 16:00 Asia/Singapore", got)
	}
}

func TestServiceRejectsInvalidTimezone(t *testing.T) {
	service, err := NewService(fixedSource{value: stdtime.Now()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.NowIn(context.Background(), "UTC+8"); err == nil {
		t.Fatal("fixed offset label unexpectedly accepted as IANA timezone")
	}
}

func TestServicePropagatesSourceFailure(t *testing.T) {
	want := errors.New("clock unavailable")
	service, err := NewService(fixedSource{err: want})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.NowUTC(context.Background()); !errors.Is(err, want) {
		t.Fatalf("NowUTC() error = %v, want %v", err, want)
	}
}

func TestStartOfDayUsesLocalCalendar(t *testing.T) {
	instant := stdtime.Date(2026, 11, 17, 8, 30, 0, 0, stdtime.UTC)
	got, err := StartOfDay(instant, "Asia/Singapore")
	if err != nil {
		t.Fatal(err)
	}
	want := stdtime.Date(2026, 11, 17, 0, 0, 0, 0, mustLocation(t, "Asia/Singapore"))
	if !got.Equal(want) {
		t.Fatalf("StartOfDay() = %s, want %s", got, want)
	}
}

func TestAddCalendarDaysRespectsDaylightSaving(t *testing.T) {
	location := mustLocation(t, "America/New_York")
	start := stdtime.Date(2026, 3, 7, 12, 0, 0, 0, location)
	got, err := AddCalendarDays(start, 1, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hour() != 12 || got.Sub(start) != 23*stdtime.Hour {
		t.Fatalf("AddCalendarDays() = %s, elapsed=%s; want local noon after 23 hours", got, got.Sub(start))
	}
}

func mustLocation(t *testing.T, name string) *stdtime.Location {
	t.Helper()
	location, err := stdtime.LoadLocation(name)
	if err != nil {
		t.Fatal(err)
	}
	return location
}
