package itinerary

import (
	"strings"
	"testing"
)

func TestResolveDayDatesFromStart(t *testing.T) {
	trip := Trip{
		Start: "2026-06-22",
		Days: []Day{
			{Day: 1, Title: "Arrive"},
			{Day: 4, Title: "Move"},
			{Day: 14, Title: "Depart"},
		},
	}
	if err := ResolveDayDates(&trip); err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-06-22", "2026-06-25", "2026-07-05"}
	for i, w := range want {
		if trip.Days[i].Date != w {
			t.Errorf("day %d date = %q, want %q", trip.Days[i].Day, trip.Days[i].Date, w)
		}
	}
}

func TestResolveDayDatesStartOverridesStored(t *testing.T) {
	trip := Trip{
		Start: "2026-06-22",
		Days: []Day{
			{Day: 1, Date: "2099-01-01", Title: "Arrive"},
			{Day: 2, Date: "2026-06-24", Title: "Stale"},
		},
	}
	if err := ResolveDayDates(&trip); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Date != "2026-06-22" {
		t.Fatalf("day 1 = %q", trip.Days[0].Date)
	}
	if trip.Days[1].Date != "2026-06-23" {
		t.Fatalf("day 2 = %q, want derived 2026-06-23", trip.Days[1].Date)
	}
}

func TestResolveDayDatesOptional(t *testing.T) {
	trip := Trip{Days: []Day{{Day: 1, Title: "No dates"}}}
	if err := ResolveDayDates(&trip); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Date != "" {
		t.Fatalf("date = %q, want empty", trip.Days[0].Date)
	}
}

func TestResolveDayDatesAfterSwap(t *testing.T) {
	trip := Trip{
		Start: "2026-06-22",
		Days: []Day{
			{Day: 1, Title: "A", Date: "2026-06-22"},
			{Day: 2, Title: "B", Date: "2026-06-23"},
		},
	}
	// Simulate swap_days content swap with stale dates traveling along.
	trip.Days[0], trip.Days[1] = trip.Days[1], trip.Days[0]
	trip.Days[0].Day, trip.Days[1].Day = 1, 2
	if err := ResolveDayDates(&trip); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Title != "B" || trip.Days[0].Date != "2026-06-22" {
		t.Fatalf("day 1 after swap: %+v", trip.Days[0])
	}
	if trip.Days[1].Title != "A" || trip.Days[1].Date != "2026-06-23" {
		t.Fatalf("day 2 after swap: %+v", trip.Days[1])
	}
}

func TestMarshalYAMLStripsDerivedDates(t *testing.T) {
	trip := Trip{
		SchemaVersion: 2,
		Trip:          "T",
		Start:         "2026-06-22",
		Places:        map[string]Place{"a": {Title: "A"}},
		Days: []Day{
			{Day: 1, Title: "One", Date: "2026-06-22", Stops: []Stop{{Place: "a"}}},
		},
	}
	if err := ResolveDayDates(&trip); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Date == "" {
		t.Fatal("expected in-memory date after resolve")
	}
	raw, err := MarshalYAML(trip)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "date:") {
		t.Fatalf("persisted YAML should omit derived dates:\n%s", raw)
	}
	if !strings.Contains(string(raw), "start:") {
		t.Fatal("expected start in YAML")
	}
	// Caller trip unchanged (MarshalYAML strips a copy).
	if trip.Days[0].Date != "2026-06-22" {
		t.Fatalf("caller date cleared: %q", trip.Days[0].Date)
	}
}

func TestDayFolderName(t *testing.T) {
	if got := DayFolderName(Day{Day: 1, Title: "Arrive"}); got != "Day 1 - Arrive" {
		t.Fatalf("no date: %q", got)
	}
	got := DayFolderName(Day{Day: 1, Date: "2026-06-22", Title: "Arrive"})
	want := "Day 1 · 22 Jun - Arrive"
	if got != want {
		t.Fatalf("with date: got %q want %q", got, want)
	}
}
