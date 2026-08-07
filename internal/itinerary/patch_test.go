package itinerary

import "testing"

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
}
