package itinerary

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplyPatchRejectsEmpty(t *testing.T) {
	trip := Trip{Trip: "T", Days: []Day{{Day: 1, Title: "A"}}}
	if err := ApplyPatch(&trip, Patch{}); err == nil {
		t.Fatal("expected empty patch error")
	}
}

func TestApplyPatchSwapAndDelete(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"a": {Title: "a", Lat: 1, Lon: 2},
			"b": {Title: "b", Lat: 3, Lon: 4},
			"c": {Title: "c", Lat: 5, Lon: 6},
		},
		Days: []Day{
			{Day: 1, Title: "A", Stops: []Stop{{Place: "a"}}},
			{Day: 2, Title: "B", Stops: []Stop{{Place: "b"}}},
			{Day: 3, Title: "C", Stops: []Stop{{Place: "c"}}},
		},
	}
	if err := ApplyPatch(&trip, Patch{SwapDays: []int{1, 3}}); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Title != "C" || trip.Days[0].Day != 1 {
		t.Fatalf("after swap day1 = %+v", trip.Days[0])
	}
	del := 2
	if err := ApplyPatch(&trip, Patch{DeleteDay: &del}); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days) != 2 || trip.Days[1].Day != 2 {
		t.Fatalf("after delete = %+v", trip.Days)
	}
}

func TestApplyPatchPlacesInfo(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"trail": {Title: "Trail", Lat: 1, Lon: 2, Type: "trailhead"},
		},
		Days: []Day{{Day: 1, Title: "Hike", Stops: []Stop{{Place: "trail"}}}},
	}
	err := ApplyPatch(&trip, Patch{
		Places: map[string]any{
			"trail": map[string]any{
				"info": map[string]any{
					"links": []map[string]string{
						{"type": "alltrails", "title": "AllTrails", "url": "https://example.com"},
					},
					"warnings": []string{"Alpine weather"},
					"logistics": map[string]any{
						"opening_hours": "Daily 10:00–17:00",
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	info := trip.Places["trail"].Info
	if info == nil || len(info.Links) != 1 || info.Links[0].URL != "https://example.com" {
		t.Fatalf("info = %+v", info)
	}
	if len(info.Warnings) != 1 {
		t.Fatalf("warnings = %v", info.Warnings)
	}
	if info.Logistics == nil || info.Logistics.OpeningHours != "Daily 10:00–17:00" {
		t.Fatalf("logistics = %+v", info.Logistics)
	}
}

func TestApplyPatchRejectsUnknownPlaceInfoFields(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"venue": {Title: "Venue", Lat: 1, Lon: 2, Type: "attraction"},
		},
		Days: []Day{{Day: 1, Title: "Day", Stops: []Stop{{Place: "venue"}}}},
	}
	err := ApplyPatch(&trip, Patch{
		Places: map[string]any{
			"venue": map[string]any{
				"info": map[string]any{
					"stats": map[string]any{
						"opening_hours": "Daily 10:00–22:30",
					},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "opening_hours") || !strings.Contains(err.Error(), "PlaceStats") {
		t.Fatalf("expected PlaceStats unknown-field error for stats.opening_hours, got %v", err)
	}
	if !strings.Contains(err.Error(), "getSchema") {
		t.Fatalf("error should point at getSchema: %v", err)
	}
	if trip.Places["venue"].Info != nil {
		t.Fatalf("info should be unchanged, got %+v", trip.Places["venue"].Info)
	}
}

func TestApplyPatchUpdateDay(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"},
		},
		Days: []Day{{Day: 1, Title: "Arrive", Notes: "Recover.", Stops: []Stop{{Place: "a"}}}},
	}
	title := "Arrive Auckland"
	notes := "Recover from flight after arriving on UA917 from SFO at 09:10 NZDT."
	hike := false
	if err := ApplyPatch(&trip, Patch{
		UpdateDay: &UpdateDay{Day: 1, Title: &title, Notes: &notes, Hike: &hike},
	}); err != nil {
		t.Fatal(err)
	}
	d := trip.Days[0]
	if d.Title != title || d.Notes != notes || d.Hike {
		t.Fatalf("day = %+v", d)
	}
	// omitted ferry unchanged
	if d.Ferry {
		t.Fatal("ferry should remain false/unchanged")
	}
}

func TestUpdateDayRejectsStopsField(t *testing.T) {
	var p Patch
	err := json.Unmarshal([]byte(`{
		"places":{"x":{"title":"X","lat":1,"lon":2,"type":"attraction"}},
		"update_day":{"day":1,"stops":[{"place":"x"}]}
	}`), &p)
	if err == nil {
		t.Fatal("expected error for update_day.stops")
	}
	if !strings.Contains(err.Error(), "upsert_stop") {
		t.Fatalf("error should point at upsert_stop: %v", err)
	}
}

func TestApplyPatchPlacesMapsURL(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"trail": {Title: "Trail", Lat: 1, Lon: 2, Type: "trailhead"},
		},
		Days: []Day{{Day: 1, Title: "Hike", Stops: []Stop{{Place: "trail"}}}},
	}
	err := ApplyPatch(&trip, Patch{
		Places: map[string]any{
			"trail": map[string]any{
				"maps_url": "https://maps.app.goo.gl/example",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if trip.Places["trail"].MapsURL != "https://maps.app.goo.gl/example" {
		t.Fatalf("maps_url = %q", trip.Places["trail"].MapsURL)
	}
}

func TestApplyPatchNewPlaceTitleUnderInfo(t *testing.T) {
	trip := Trip{
		Trip:   "T",
		Places: map[string]Place{"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"}},
		Days:   []Day{{Day: 1, Title: "D", Route: []Stop{{Place: "a"}}}},
	}
	err := ApplyPatch(&trip, Patch{
		Places: map[string]any{
			"greymouth": map[string]any{
				"info": map[string]any{
					"title":      "Greymouth",
					"highlights": []string{"West Coast town"},
				},
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "not under info") {
		t.Fatalf("expected title-under-info error, got %v", err)
	}
}

func TestApplyPatchNewPlaceInfoString(t *testing.T) {
	trip := Trip{
		Trip:   "T",
		Places: map[string]Place{"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"}},
		Days:   []Day{{Day: 1, Title: "D", Route: []Stop{{Place: "a"}}}},
	}
	err := ApplyPatch(&trip, Patch{
		Places: map[string]any{
			"greymouth": map[string]any{
				"info": "A town on the West Coast",
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "info must be an object") {
		t.Fatalf("expected info-string error, got %v", err)
	}
}

func TestApplyPatchNewPlaceMissingTitle(t *testing.T) {
	trip := Trip{
		Trip:   "T",
		Places: map[string]Place{"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"}},
		Days:   []Day{{Day: 1, Title: "D", Route: []Stop{{Place: "a"}}}},
	}
	err := ApplyPatch(&trip, Patch{
		Places: map[string]any{
			"greymouth": map[string]any{
				"lat":  -42.45,
				"lon":  171.21,
				"type": "overnight",
			},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "top-level title is required") {
		t.Fatalf("expected missing-title error, got %v", err)
	}
}

func TestApplyPatchDaysRouteClearsStaleMapsURL(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"},
			"b": {Title: "B", Lat: 3, Lon: 4, Type: "via"},
			"c": {Title: "C", Lat: 5, Lon: 6, Type: "overnight"},
		},
		Days: []Day{{
			Day:   1,
			Title: "A → C",
			Route: []Stop{
				{Place: "a", Type: "overnight", MapsURL: "https://maps/?q=A"},
				{Place: "b", Type: "via", MapsURL: "https://maps/?q=WRONG"},
				{Place: "c", Type: "overnight", MapsURL: "https://maps/?q=C"},
			},
		}},
	}
	if err := ApplyPatch(&trip, Patch{
		Days: map[string]any{
			"1": map[string]any{
				"route": []map[string]string{
					{"place": "a", "type": "overnight"},
					{"place": "b", "type": "via"},
					{"place": "c", "type": "overnight"},
				},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Route[1].MapsURL != "" {
		t.Fatalf("stale maps_url kept on mid stop: %q", trip.Days[0].Route[1].MapsURL)
	}
	if trip.Days[0].Route[0].MapsURL != "" || trip.Days[0].Route[2].MapsURL != "" {
		t.Fatalf("stale maps_url kept on ends: %+v", trip.Days[0].Route)
	}
}

func TestApplyPatchOvernightViaDaysRoutes(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"},
			"b": {Title: "B", Lat: 3, Lon: 4, Type: "overnight"},
			"c": {Title: "C", Lat: 5, Lon: 6, Type: "overnight"},
			"d": {Title: "D", Lat: 7, Lon: 8, Type: "overnight"},
		},
		Days: []Day{
			{Day: 1, Title: "A → B", Route: []Stop{{Place: "a"}, {Place: "b"}}},
			{Day: 2, Title: "B → C", Route: []Stop{{Place: "b"}, {Place: "c"}}},
		},
	}
	if err := ApplyPatch(&trip, Patch{
		Places: map[string]any{
			"d": map[string]any{"title": "D", "lat": 7.0, "lon": 8.0, "type": "overnight"},
		},
		Days: map[string]any{
			"1": map[string]any{
				"title": "A → D",
				"route": []map[string]string{{"place": "a"}, {"place": "d"}},
			},
			"2": map[string]any{
				"title": "D → C",
				"route": []map[string]string{{"place": "d"}, {"place": "c"}},
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Title != "A → D" || trip.Days[0].Route[1].Place != "d" {
		t.Fatalf("day1 = %+v", trip.Days[0])
	}
	if trip.Days[1].Title != "D → C" || trip.Days[1].Route[0].Place != "d" {
		t.Fatalf("day2 = %+v", trip.Days[1])
	}
}

func TestApplyPatchUpsertUnknownPlaceHint(t *testing.T) {
	trip := Trip{
		Trip:   "T",
		Places: map[string]Place{"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"}},
		Days:   []Day{{Day: 1, Title: "D", Route: []Stop{{Place: "a"}}}},
	}
	err := ApplyPatch(&trip, Patch{
		UpsertStop: &UpsertStop{Day: 1, List: "route", Place: "Greymouth"},
	})
	if err == nil || !strings.Contains(err.Error(), "kebab-case") {
		t.Fatalf("expected unknown-place hint, got %v", err)
	}
}

func TestApplyPatchUpsertRemoveStop(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"},
			"b": {Title: "B", Lat: 3, Lon: 4, Type: "attraction"},
		},
		Days: []Day{{Day: 1, Title: "D", Route: []Stop{{Place: "a"}}}},
	}
	if err := ApplyPatch(&trip, Patch{
		UpsertStop: &UpsertStop{Day: 1, List: "stops", Place: "b"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Stops) != 1 || trip.Days[0].Stops[0].Place != "b" {
		t.Fatalf("stops = %+v", trip.Days[0].Stops)
	}
	if err := ApplyPatch(&trip, Patch{
		RemoveStop: &RemoveStop{Day: 1, List: "stops", Place: "b"},
	}); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Stops) != 0 {
		t.Fatalf("expected empty stops, got %+v", trip.Days[0].Stops)
	}
	if err := ApplyPatch(&trip, Patch{
		RemoveStop: &RemoveStop{Day: 1, List: "stops", Place: "missing"},
	}); err == nil {
		t.Fatal("expected error removing missing place")
	}
}

func TestApplyPatchRemoveStopsByTitleAndIDs(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"a": {Title: "Alpha Pub", Lat: 1, Lon: 2, Type: "pub"},
			"b": {Title: "Beta Lounge", Lat: 3, Lon: 4, Type: "pub"},
			"c": {Title: "Camp", Lat: 5, Lon: 6, Type: "overnight"},
		},
		Days: []Day{{
			Day:   1,
			Title: "D",
			Stops: []Stop{{Place: "a"}, {Place: "b"}, {Place: "c"}},
		}},
	}
	if err := ApplyPatch(&trip, Patch{
		RemoveStop: &RemoveStop{
			Day:    1,
			List:   "stops",
			Places: []string{"Alpha Pub", "b"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days[0].Stops) != 1 || trip.Days[0].Stops[0].Place != "c" {
		t.Fatalf("stops = %+v", trip.Days[0].Stops)
	}
}
