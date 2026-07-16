package itinerary

import "testing"

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

func TestResolveDayDatesExplicitWins(t *testing.T) {
	trip := Trip{
		Start: "2026-06-22",
		Days: []Day{
			{Day: 1, Title: "Arrive"},
			{Day: 2, Date: "2026-06-24", Title: "Rest"},
		},
	}
	if err := ResolveDayDates(&trip); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Date != "2026-06-22" {
		t.Fatalf("day 1 = %q", trip.Days[0].Date)
	}
	if trip.Days[1].Date != "2026-06-24" {
		t.Fatalf("day 2 override = %q, want 2026-06-24", trip.Days[1].Date)
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
