package bundle

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/routebuild"
)

func TestBuildTripBundleStraight(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bundle")

	km := 19.4
	trip := itinerary.Trip{
		Trip:        "Test Trip",
		Description: "desc",
		Places: map[string]itinerary.Place{
			"a": {Title: "A", Type: "overnight", Lat: 1, Lon: 2},
			"b": {Title: "B", Type: "overnight", Lat: 3, Lon: 4},
			"trail": {
				Title: "Trail", Type: "trailhead", Lat: 5, Lon: 6,
				Info: &itinerary.PlaceInfo{
					Links: []itinerary.PlaceLink{{Type: "alltrails", Title: "AllTrails", URL: "https://example.com"}},
					Stats: &itinerary.PlaceStats{DistanceKm: &km, Duration: "7h"},
				},
			},
			"hut": {Title: "Hut", Type: "hut", Lat: 7, Lon: 8},
		},
		Days: []itinerary.Day{
			{
				Day: 1, Title: "Start",
				Route: []itinerary.Stop{
					{Place: "a"},
					{Place: "b"},
				},
			},
			{
				Day: 2, Title: "Hike", Hike: true,
				Route: []itinerary.Stop{
					{Place: "trail"},
					{Place: "hut"},
				},
			},
		},
	}

	err := Build(context.Background(), trip, "test", "itineraries", out, routebuild.RouteOptions{Mode: "straight", Units: "km", CoordPrecision: 6})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	tripPath := filepath.Join(out, "trip.json")
	b, err := os.ReadFile(tripPath)
	if err != nil {
		t.Fatalf("read trip.json: %v", err)
	}
	var tj TripJSON
	if err := json.Unmarshal(b, &tj); err != nil {
		t.Fatalf("parse trip.json: %v", err)
	}
	if tj.Title != "Test Trip" || len(tj.Days) != 2 {
		t.Fatalf("trip.json = %+v", tj)
	}
	if tj.Days[1].Kind != "hike" {
		t.Fatalf("day2 kind = %q", tj.Days[1].Kind)
	}
	foundInfo := false
	for _, s := range tj.Days[1].Stops {
		if s.Place == "trail" && s.Info != nil && len(s.Info.Links) == 1 {
			foundInfo = true
		}
	}
	if !foundInfo {
		t.Fatalf("expected trail info in day2 stops: %+v", tj.Days[1].Stops)
	}

	index := filepath.Join(out, "index.html")
	ib, err := os.ReadFile(index)
	if err != nil {
		t.Fatalf("index.html: %v", err)
	}
	if !strings.Contains(string(ib), "app.js") {
		t.Fatalf("index.html missing viewer assets")
	}
}
