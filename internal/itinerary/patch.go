package itinerary

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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
// Does not accept route/stops — use UpsertStop or replaceDayRoutes.
type UpdateDay struct {
	Day          int     `json:"day"`
	Title        *string `json:"title,omitempty"`
	Notes        *string `json:"notes,omitempty"`
	Hike         *bool   `json:"hike,omitempty"`
	Ferry        *bool   `json:"ferry,omitempty"`
	Photo        *string `json:"photo,omitempty"`
	PhotoCaption *string `json:"photo_caption,omitempty"`
}

// UnmarshalJSON rejects route/stops and other unknown fields so agents cannot
// silently no-op when they put stops under update_day instead of upsert_stop.
func (u *UpdateDay) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	allowed := map[string]struct{}{
		"day": {}, "title": {}, "notes": {}, "hike": {}, "ferry": {},
		"photo": {}, "photo_caption": {},
	}
	for k := range raw {
		if _, ok := allowed[k]; ok {
			continue
		}
		if k == "stops" || k == "route" {
			return fmt.Errorf("update_day: %s is not supported — use upsert_stop with list \"route\" or \"stops\" (create the place under places in the same patch); for overnight/endpoint changes use replaceDayRoutes", k)
		}
		return fmt.Errorf("update_day: unknown field %q (allowed: day, title, notes, hike, ferry, photo, photo_caption)", k)
	}
	type alias UpdateDay
	var a alias
	if err := json.Unmarshal(data, &a); err != nil {
		return err
	}
	*u = UpdateDay(a)
	return nil
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

// RemoveStop removes route/stop refs from a day.
// Use Place for one id, or Places for several. Values may be place ids or exact titles.
type RemoveStop struct {
	Day    int      `json:"day"`
	List   string   `json:"list"` // "route" or "stops"; empty = both
	Place  string   `json:"place,omitempty"`
	Places []string `json:"places,omitempty"`
}

// InsertDay inserts a day after the given day number.
type InsertDay struct {
	After int             `json:"after"`
	Day   json.RawMessage `json:"day"`
}

// Empty reports whether the patch carries no operations.
func (p Patch) Empty() bool {
	return len(p.SwapDays) == 0 &&
		len(p.Days) == 0 &&
		p.UpdateDay == nil &&
		len(p.Places) == 0 &&
		p.UpsertStop == nil &&
		p.RemoveStop == nil &&
		p.InsertDay == nil &&
		p.DeleteDay == nil
}

// ApplyPatch mutates t in place.
func ApplyPatch(t *Trip, p Patch) error {
	if p.Empty() {
		return fmt.Errorf("patch is empty")
	}
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
			if !ok && strings.TrimSpace(merged.Title) == "" {
				return fmt.Errorf("places.%s: top-level title is required for new places (not under info); also set lat, lon, type", id)
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
		// encoding/json reuses slice backing arrays; omitted stop fields would
		// otherwise keep stale maps_url/type from the previous route/stops.
		if dayPatchHasKey(raw, "route") {
			cur.Route = nil
		}
		if dayPatchHasKey(raw, "stops") {
			cur.Stops = nil
		}
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

func dayPatchHasKey(raw any, key string) bool {
	m, ok := raw.(map[string]any)
	if !ok {
		return false
	}
	_, ok = m[key]
	return ok
}

// checkPlacePatchShape catches common agent mistakes before silent field drops.
func checkPlacePatchShape(raw any) error {
	m, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	info, hasInfo := m["info"]
	if !hasInfo {
		return nil
	}
	switch v := info.(type) {
	case string:
		return fmt.Errorf("info must be an object, not a string; put title/lat/lon/type on the place, and enrichment (highlights, links, …) under info")
	case map[string]any:
		if _, has := v["title"]; has {
			if _, top := m["title"]; !top {
				return fmt.Errorf("title belongs on the place object, not under info (use places.<id>.title)")
			}
		}
	}
	return nil
}

func mergePlace(cur Place, raw any) (Place, error) {
	if err := checkPlacePatchShape(raw); err != nil {
		return cur, err
	}
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
	if err := decodeJSONStrict(b, &patch, "PlacePatch", placePatchFields); err != nil {
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
		Source     *PlaceSource    `json:"source"`
		Links      []PlaceLink     `json:"links"`
		Stats      json.RawMessage `json:"stats"`
		Logistics  json.RawMessage `json:"logistics"`
		Facilities json.RawMessage `json:"facilities"`
		Warnings   []string        `json:"warnings"`
		Highlights []string        `json:"highlights"`
	}
	if err := decodeJSONStrict(raw, &patch, "PlaceInfo", placeInfoFields); err != nil {
		return nil, err
	}
	out := *cur
	if patch.Source != nil {
		out.Source = patch.Source
	}
	if patch.Links != nil {
		out.Links = patch.Links
	}
	if len(patch.Stats) > 0 && string(patch.Stats) != "null" {
		var stats PlaceStats
		if err := decodeJSONStrict(patch.Stats, &stats, "PlaceStats", placeStatsFields); err != nil {
			return nil, err
		}
		out.Stats = mergeStats(out.Stats, &stats)
	}
	if len(patch.Logistics) > 0 && string(patch.Logistics) != "null" {
		var logistics PlaceLogistics
		if err := decodeJSONStrict(patch.Logistics, &logistics, "PlaceLogistics", placeLogisticsFields); err != nil {
			return nil, err
		}
		out.Logistics = mergeLogistics(out.Logistics, &logistics)
	}
	if len(patch.Facilities) > 0 && string(patch.Facilities) != "null" {
		var facilities PlaceFacilities
		if err := decodeJSONStrict(patch.Facilities, &facilities, "PlaceFacilities", []string{"toilets", "drinking_water"}); err != nil {
			return nil, err
		}
		out.Facilities = mergeFacilities(out.Facilities, &facilities)
	}
	if patch.Warnings != nil {
		out.Warnings = patch.Warnings
	}
	if patch.Highlights != nil {
		out.Highlights = patch.Highlights
	}
	return &out, nil
}

var placeInfoFields = []string{"source", "links", "stats", "logistics", "facilities", "warnings", "highlights"}
var placePatchFields = []string{"title", "lat", "lon", "type", "notes", "photo", "photo_caption", "maps_url", "info"}
var placeStatsFields = []string{"distance_km", "duration", "ascent_m", "difficulty"}
var placeLogisticsFields = []string{"parking", "opening_hours", "booking_required"}

func decodeJSONStrict(raw json.RawMessage, dest any, schemaName string, allowed []string) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "unknown field") && len(allowed) > 0 {
			return fmt.Errorf("%s: %w (allowed: %s; see getSchema schemas.%s)", schemaName, err, strings.Join(allowed, ", "), schemaName)
		}
		return fmt.Errorf("%s: %w", schemaName, err)
	}
	return nil
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
	if patch.OpeningHours != "" {
		out.OpeningHours = patch.OpeningHours
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
		return fmt.Errorf("upsert_stop: unknown place %q — create it in the same patch under places with a kebab-case id and top-level title/lat/lon/type", u.Place)
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
	refs := make([]string, 0, 1+len(r.Places))
	if strings.TrimSpace(r.Place) != "" {
		refs = append(refs, r.Place)
	}
	for _, p := range r.Places {
		if strings.TrimSpace(p) != "" {
			refs = append(refs, p)
		}
	}
	if len(refs) == 0 {
		return fmt.Errorf("remove_stop: place or places is required")
	}
	switch r.List {
	case "route", "stops", "":
	default:
		return fmt.Errorf("remove_stop: list must be \"route\", \"stops\", or empty")
	}

	for _, ref := range refs {
		id, err := resolveDayPlaceRef(t, i, ref, r.List)
		if err != nil {
			return fmt.Errorf("remove_stop: %w", err)
		}
		removed := false
		switch r.List {
		case "route":
			before := len(t.Days[i].Route)
			t.Days[i].Route = filterPlace(t.Days[i].Route, id)
			removed = len(t.Days[i].Route) < before
		case "stops":
			before := len(t.Days[i].Stops)
			t.Days[i].Stops = filterPlace(t.Days[i].Stops, id)
			removed = len(t.Days[i].Stops) < before
		case "":
			br, bs := len(t.Days[i].Route), len(t.Days[i].Stops)
			t.Days[i].Route = filterPlace(t.Days[i].Route, id)
			t.Days[i].Stops = filterPlace(t.Days[i].Stops, id)
			removed = len(t.Days[i].Route) < br || len(t.Days[i].Stops) < bs
		}
		if !removed {
			return fmt.Errorf("remove_stop: place %q not found on day %d", ref, r.Day)
		}
	}
	return nil
}

// resolveDayPlaceRef maps a place id or exact title to a place id on the day.
func resolveDayPlaceRef(t *Trip, dayIdx int, ref, list string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("empty place")
	}
	candidates := make([]Stop, 0, len(t.Days[dayIdx].Route)+len(t.Days[dayIdx].Stops))
	switch list {
	case "route":
		candidates = append(candidates, t.Days[dayIdx].Route...)
	case "stops":
		candidates = append(candidates, t.Days[dayIdx].Stops...)
	default:
		candidates = append(candidates, t.Days[dayIdx].Route...)
		candidates = append(candidates, t.Days[dayIdx].Stops...)
	}
	for _, s := range candidates {
		if s.Place == ref {
			return ref, nil
		}
	}
	want := strings.ToLower(ref)
	var matches []string
	seen := map[string]bool{}
	for _, s := range candidates {
		p, ok := t.Places[s.Place]
		if !ok || seen[s.Place] {
			continue
		}
		if strings.ToLower(strings.TrimSpace(p.Title)) == want {
			matches = append(matches, s.Place)
			seen[s.Place] = true
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("place %q not found on day %d (use place id from get_day)", ref, t.Days[dayIdx].Day)
	default:
		return "", fmt.Errorf("place title %q is ambiguous on day %d", ref, t.Days[dayIdx].Day)
	}
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
