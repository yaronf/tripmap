package itinerary

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// ResolveDayDates fills missing day.Date values from trip.Start + (day-1).
// Explicit per-day dates win. Dates are optional; empty stay empty.
func ResolveDayDates(t *Trip) error {
	var start time.Time
	var hasStart bool
	if t.Start != "" {
		s, err := time.Parse(dateLayout, t.Start)
		if err != nil {
			return fmt.Errorf("trip start %q: use YYYY-MM-DD", t.Start)
		}
		start = s
		hasStart = true
	}

	for i := range t.Days {
		d := &t.Days[i]
		if d.Date != "" {
			if _, err := time.Parse(dateLayout, d.Date); err != nil {
				return fmt.Errorf("day %d date %q: use YYYY-MM-DD", d.Day, d.Date)
			}
			continue
		}
		if !hasStart || d.Day < 1 {
			continue
		}
		d.Date = start.AddDate(0, 0, d.Day-1).Format(dateLayout)
	}
	return nil
}

// FormatDayDateShort returns a compact calendar label, e.g. "22 Jun".
func FormatDayDateShort(iso string) string {
	t, err := time.Parse(dateLayout, iso)
	if err != nil {
		return iso
	}
	return t.Format("2 Jan")
}

// DayFolderName is the KML folder label for a day.
func DayFolderName(d Day) string {
	if d.Date == "" {
		return fmt.Sprintf("Day %d - %s", d.Day, d.Title)
	}
	return fmt.Sprintf("Day %d · %s - %s", d.Day, FormatDayDateShort(d.Date), d.Title)
}
