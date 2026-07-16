package itinerary

import (
	"encoding/json"
	"fmt"
)

// Patch is a structured itinerary mutation (Custom GPT Actions).
type Patch struct {
	SwapDays  []int          `json:"swap_days,omitempty"`
	Days      map[string]any `json:"days,omitempty"` // day number string -> partial day
	InsertDay *InsertDay     `json:"insert_day,omitempty"`
	DeleteDay *int           `json:"delete_day,omitempty"`
}

// InsertDay inserts a day after the given day number.
type InsertDay struct {
	After int             `json:"after"`
	Day   json.RawMessage `json:"day"`
}

// ApplyPatch mutates t in place.
func ApplyPatch(t *Trip, p Patch) error {
	if len(p.SwapDays) == 2 {
		a, b := p.SwapDays[0], p.SwapDays[1]
		ia, ib := dayIndex(*t, a), dayIndex(*t, b)
		if ia < 0 || ib < 0 {
			return fmt.Errorf("swap_days: day not found")
		}
		t.Days[ia], t.Days[ib] = t.Days[ib], t.Days[ia]
		t.Days[ia].Day, t.Days[ib].Day = a, b
	} else if len(p.SwapDays) != 0 {
		return fmt.Errorf("swap_days must have exactly two day numbers")
	}

	for key, raw := range p.Days {
		var n int
		if _, err := fmt.Sscanf(key, "%d", &n); err != nil {
			return fmt.Errorf("days key %q: want day number", key)
		}
		i := dayIndex(*t, n)
		if i < 0 {
			return fmt.Errorf("days.%d: not found", n)
		}
		b, err := json.Marshal(raw)
		if err != nil {
			return err
		}
		// Merge onto existing day via JSON round-trip of partial fields.
		cur := t.Days[i]
		if err := json.Unmarshal(b, &cur); err != nil {
			return fmt.Errorf("days.%d: %w", n, err)
		}
		cur.Day = n
		t.Days[i] = cur
	}

	if p.DeleteDay != nil {
		i := dayIndex(*t, *p.DeleteDay)
		if i < 0 {
			return fmt.Errorf("delete_day: day %d not found", *p.DeleteDay)
		}
		t.Days = append(t.Days[:i], t.Days[i+1:]...)
		renumberDays(t)
	}

	if p.InsertDay != nil {
		var day Day
		if err := json.Unmarshal(p.InsertDay.Day, &day); err != nil {
			return fmt.Errorf("insert_day.day: %w", err)
		}
		after := p.InsertDay.After
		i := dayIndex(*t, after)
		if after != 0 && i < 0 {
			return fmt.Errorf("insert_day.after: day %d not found", after)
		}
		insertAt := 0
		if after != 0 {
			insertAt = i + 1
		}
		t.Days = append(t.Days[:insertAt], append([]Day{day}, t.Days[insertAt:]...)...)
		renumberDays(t)
	}

	return ValidateBasic(*t)
}

func dayIndex(t Trip, n int) int {
	for i, d := range t.Days {
		if d.Day == n {
			return i
		}
	}
	return -1
}

func renumberDays(t *Trip) {
	for i := range t.Days {
		t.Days[i].Day = i + 1
	}
}
