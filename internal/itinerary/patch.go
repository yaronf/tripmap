package itinerary

import (
	"encoding/json"
	"fmt"
)

// Patch is a structured itinerary mutation (agent API / MCP).
type Patch struct {
	SwapDays   []int          `json:"swap_days,omitempty"`
	Days       map[string]any `json:"days,omitempty"` // day number string -> partial day
	UpdateDay  *UpdateDay     `json:"update_day,omitempty"`
	Places     map[string]any `json:"places,omitempty"` // place id -> partial place (info deep-merged)
	UpsertStop *UpsertStop    `json:"upsert_stop,omitempty"`
	RemoveStop *RemoveStop    `json:"remove_stop,omitempty"`
	InsertDay  *InsertDay     `json:"insert_day,omitempty"`
	DeleteDay  *int           `json:"delete_day,omitempty"`
}

// UpdateDay partially updates one existing day's narrative/flags.
// Omitted pointer fields are left unchanged.
type UpdateDay struct {
	Day          int     `json:"day"`
	Title        *string `json:"title,omitempty"`
	Notes        *string `json:"notes,omitempty"`
	Hike         *bool   `json:"hike,omitempty"`
	Ferry        *bool   `json:"ferry,omitempty"`
	Photo        *string `json:"photo,omitempty"`
	PhotoCaption *string `json:"photo_caption,omitempty"`
}

// UpsertStop adds or replaces a route/stop ref on a day by place id.
type UpsertStop struct {
	Day     int    `json:"day"`
	List    string `json:"list"` // "route" or "stops"
	Place   string `json:"place"`
	Type    string `json:"type,omitempty"`
	Notes   string `json:"notes,omitempty"`
	MapsURL string `json:"maps_url,omitempty"`
}

// RemoveStop removes a route/stop ref matching place id from a day.
type RemoveStop struct {
	Day   int    `json:"day"`
	List  string `json:"list"` // "route" or "stops"; empty = both
	Place string `json:"place"`
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

	if len(p.Places) > 0 {
		if t.Places == nil {
			t.Places = map[string]Place{}
		}
		for id, raw := range p.Places {
			if id == "" {
				return fmt.Errorf("places: empty id")
			}
			cur, ok := t.Places[id]
			if !ok {
				cur = Place{}
			}
			merged, err := mergePlace(cur, raw)
			if err != nil {
				return fmt.Errorf("places.%s: %w", id, err)
			}
			t.Places[id] = merged
		}
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
		cur := t.Days[i]
		if err := json.Unmarshal(b, &cur); err != nil {
			return fmt.Errorf("days.%d: %w", n, err)
		}
		cur.Day = n
		t.Days[i] = cur
	}

	if p.UpdateDay != nil {
		if err := applyUpdateDay(t, *p.UpdateDay); err != nil {
			return err
		}
	}

	if p.UpsertStop != nil {
		if err := applyUpsertStop(t, *p.UpsertStop); err != nil {
			return err
		}
	}
	if p.RemoveStop != nil {
		if err := applyRemoveStop(t, *p.RemoveStop); err != nil {
			return err
		}
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

func mergePlace(cur Place, raw any) (Place, error) {
	b, err := json.Marshal(raw)
	if err != nil {
		return cur, err
	}
	var patch struct {
		Title        *string          `json:"title"`
		Lat          *float64         `json:"lat"`
		Lon          *float64         `json:"lon"`
		Type         *string          `json:"type"`
		Notes        *string          `json:"notes"`
		Photo        *string          `json:"photo"`
		PhotoCaption *string          `json:"photo_caption"`
		MapsURL      *string          `json:"maps_url"`
		Info         *json.RawMessage `json:"info"`
	}
	if err := json.Unmarshal(b, &patch); err != nil {
		return cur, err
	}
	if patch.Title != nil {
		cur.Title = *patch.Title
	}
	if patch.Lat != nil {
		cur.Lat = *patch.Lat
	}
	if patch.Lon != nil {
		cur.Lon = *patch.Lon
	}
	if patch.Type != nil {
		cur.Type = *patch.Type
	}
	if patch.Notes != nil {
		cur.Notes = *patch.Notes
	}
	if patch.Photo != nil {
		cur.Photo = *patch.Photo
	}
	if patch.PhotoCaption != nil {
		cur.PhotoCaption = *patch.PhotoCaption
	}
	if patch.MapsURL != nil {
		cur.MapsURL = *patch.MapsURL
	}
	if patch.Info != nil {
		merged, err := mergePlaceInfo(cur.Info, *patch.Info)
		if err != nil {
			return cur, err
		}
		cur.Info = merged
	}
	return cur, nil
}

func mergePlaceInfo(cur *PlaceInfo, raw json.RawMessage) (*PlaceInfo, error) {
	if cur == nil {
		cur = &PlaceInfo{}
	}
	var patch struct {
		Source     *PlaceSource     `json:"source"`
		Links      []PlaceLink      `json:"links"`
		Stats      *PlaceStats      `json:"stats"`
		Logistics  *PlaceLogistics  `json:"logistics"`
		Facilities *PlaceFacilities `json:"facilities"`
		Warnings   []string         `json:"warnings"`
		Highlights []string         `json:"highlights"`
	}
	if err := json.Unmarshal(raw, &patch); err != nil {
		return nil, err
	}
	out := *cur
	if patch.Source != nil {
		out.Source = patch.Source
	}
	if patch.Links != nil {
		out.Links = patch.Links
	}
	if patch.Stats != nil {
		out.Stats = mergeStats(out.Stats, patch.Stats)
	}
	if patch.Logistics != nil {
		out.Logistics = mergeLogistics(out.Logistics, patch.Logistics)
	}
	if patch.Facilities != nil {
		out.Facilities = mergeFacilities(out.Facilities, patch.Facilities)
	}
	if patch.Warnings != nil {
		out.Warnings = patch.Warnings
	}
	if patch.Highlights != nil {
		out.Highlights = patch.Highlights
	}
	return &out, nil
}

func mergeStats(cur, patch *PlaceStats) *PlaceStats {
	if cur == nil {
		cur = &PlaceStats{}
	}
	out := *cur
	if patch.DistanceKm != nil {
		out.DistanceKm = patch.DistanceKm
	}
	if patch.Duration != "" {
		out.Duration = patch.Duration
	}
	if patch.AscentM != nil {
		out.AscentM = patch.AscentM
	}
	if patch.Difficulty != "" {
		out.Difficulty = patch.Difficulty
	}
	return &out
}

func mergeLogistics(cur, patch *PlaceLogistics) *PlaceLogistics {
	if cur == nil {
		cur = &PlaceLogistics{}
	}
	out := *cur
	if patch.Parking != "" {
		out.Parking = patch.Parking
	}
	if patch.BookingRequired != nil {
		out.BookingRequired = patch.BookingRequired
	}
	return &out
}

func mergeFacilities(cur, patch *PlaceFacilities) *PlaceFacilities {
	if cur == nil {
		cur = &PlaceFacilities{}
	}
	out := *cur
	if patch.Toilets != nil {
		out.Toilets = patch.Toilets
	}
	if patch.DrinkingWater != nil {
		out.DrinkingWater = patch.DrinkingWater
	}
	return &out
}

func applyUpdateDay(t *Trip, u UpdateDay) error {
	if u.Day < 1 {
		return fmt.Errorf("update_day: day is required")
	}
	i := dayIndex(*t, u.Day)
	if i < 0 {
		return fmt.Errorf("update_day: day %d not found", u.Day)
	}
	d := &t.Days[i]
	if u.Title != nil {
		d.Title = *u.Title
	}
	if u.Notes != nil {
		d.Notes = *u.Notes
	}
	if u.Hike != nil {
		d.Hike = *u.Hike
	}
	if u.Ferry != nil {
		d.Ferry = *u.Ferry
	}
	if u.Photo != nil {
		d.Photo = *u.Photo
	}
	if u.PhotoCaption != nil {
		d.PhotoCaption = *u.PhotoCaption
	}
	return nil
}

func applyUpsertStop(t *Trip, u UpsertStop) error {
	i := dayIndex(*t, u.Day)
	if i < 0 {
		return fmt.Errorf("upsert_stop: day %d not found", u.Day)
	}
	if u.Place == "" {
		return fmt.Errorf("upsert_stop: place is required")
	}
	if _, ok := t.Places[u.Place]; !ok {
		return fmt.Errorf("upsert_stop: unknown place %q", u.Place)
	}
	ref := Stop{Place: u.Place, Type: u.Type, Notes: u.Notes, MapsURL: u.MapsURL}
	switch u.List {
	case "route":
		t.Days[i].Route = upsertInList(t.Days[i].Route, ref)
	case "stops":
		t.Days[i].Stops = upsertInList(t.Days[i].Stops, ref)
	default:
		return fmt.Errorf("upsert_stop: list must be \"route\" or \"stops\"")
	}
	return nil
}

func upsertInList(list []Stop, ref Stop) []Stop {
	for i := range list {
		if list[i].Place == ref.Place {
			list[i] = ref
			return list
		}
	}
	return append(list, ref)
}

func applyRemoveStop(t *Trip, r RemoveStop) error {
	i := dayIndex(*t, r.Day)
	if i < 0 {
		return fmt.Errorf("remove_stop: day %d not found", r.Day)
	}
	if r.Place == "" {
		return fmt.Errorf("remove_stop: place is required")
	}
	switch r.List {
	case "route":
		t.Days[i].Route = filterPlace(t.Days[i].Route, r.Place)
	case "stops":
		t.Days[i].Stops = filterPlace(t.Days[i].Stops, r.Place)
	case "":
		t.Days[i].Route = filterPlace(t.Days[i].Route, r.Place)
		t.Days[i].Stops = filterPlace(t.Days[i].Stops, r.Place)
	default:
		return fmt.Errorf("remove_stop: list must be \"route\", \"stops\", or empty")
	}
	return nil
}

func filterPlace(list []Stop, place string) []Stop {
	out := list[:0]
	for _, s := range list {
		if s.Place != place {
			out = append(out, s)
		}
	}
	return out
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
