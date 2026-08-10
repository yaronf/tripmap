package itinerary

import (
	"fmt"
	"time"
)

const dateLayout = "2006-01-02"

// ResolveDayDates sets each day's Date for display/bundle use.
//
// When trip.Start is set, dates are always derived as start + (day − 1).
// Stored per-day date values are ignored (they are not persisted when Start
// is set; see stripDerivedDayDates).
//
// When Start is empty, optional explicit per-day dates are validated and kept.
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
		if hasStart {
			if d.Day < 1 {
				continue
			}
			d.Date = start.AddDate(0, 0, d.Day-1).Format(dateLayout)
			continue
		}
		if d.Date == "" {
			continue
		}
		if _, err := time.Parse(dateLayout, d.Date); err != nil {
			return fmt.Errorf("day %d date %q: use YYYY-MM-DD", d.Day, d.Date)
		}
	}
	return nil
}

// stripDerivedDayDates clears per-day Date when trip.Start is set so YAML
// does not persist values that must stay derived from Start + day number.
func stripDerivedDayDates(t *Trip) {
	if t == nil || t.Start == "" {
		return
	}
	for i := range t.Days {
		t.Days[i].Date = ""
	}
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
