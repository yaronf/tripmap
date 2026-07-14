package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildTripBundleStraight(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bundle")

	trip := Trip{
		Trip:        "Test Trip",
		Description: "desc",
		Days: []Day{
			{
				Day: 1, Title: "Start",
				Route: []Stop{
					{Name: "A", Type: "overnight", Lat: 1, Lon: 2},
					{Name: "B", Type: "overnight", Lat: 3, Lon: 4},
				},
			},
			{
				Day: 2, Title: "Hike", Hike: true,
				Route: []Stop{
					{Name: "Trail", Type: "trailhead", Lat: 5, Lon: 6},
					{Name: "Hut", Type: "hut", Lat: 7, Lon: 8},
				},
			},
		},
	}

	err := buildTripBundle(context.Background(), trip, "itineraries/test.yaml", out, straightOpts("straight"))
	if err != nil {
		t.Fatalf("buildTripBundle: %v", err)
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
	if tj.ID != "test" || tj.Title != "Test Trip" || len(tj.Days) != 2 {
		t.Fatalf("trip.json = %+v", tj)
	}
	if tj.Days[0].Geo != "geo/day-01.json" || tj.Days[0].Kind != "drive" {
		t.Fatalf("day1 = %+v", tj.Days[0])
	}
	if tj.Days[0].DriveDist <= 0 {
		t.Fatalf("day1 drive_dist = %v, want haversine length", tj.Days[0].DriveDist)
	}
	if tj.Units != "km" {
		t.Fatalf("units = %q, want km", tj.Units)
	}
	if tj.Days[0].DriveMin != 0 {
		t.Fatalf("day1 drive_min = %d, want 0 for straight routing", tj.Days[0].DriveMin)
	}
	if tj.Days[1].Kind != "hike" {
		t.Fatalf("day2 kind = %q", tj.Days[1].Kind)
	}

	for _, name := range []string{"index.html", "app.js", "style.css", "sw.js", "icon.svg", "manifest.webmanifest", "geo/day-01.json", "geo/day-02.json"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	indexHTML, err := os.ReadFile(filepath.Join(out, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(indexHTML), "Test Trip Itinerary") {
		t.Fatalf("index.html title missing trip name:\n%s", indexHTML)
	}
	if !strings.Contains(string(indexHTML), `property="og:title"`) {
		t.Fatalf("index.html missing og:title")
	}
	if strings.Contains(string(indexHTML), "<title>Trip</title>") {
		t.Fatalf("index.html still has placeholder title")
	}

	geoB, err := os.ReadFile(filepath.Join(out, "geo/day-01.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fc geoFeatureCollection
	if err := json.Unmarshal(geoB, &fc); err != nil {
		t.Fatal(err)
	}
	if fc.Type != "FeatureCollection" || len(fc.Features) < 3 {
		t.Fatalf("geo features = %+v", fc)
	}
}

func TestRunBundleOnly(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "bundle")
	err := run([]string{
		"-input", filepath.Join("testdata", "itinerary.yaml"),
		"-bundle", out,
		"-route", "straight",
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "trip.json")); err != nil {
		t.Fatal(err)
	}
}

func TestDistanceInUnits(t *testing.T) {
	const meters = 160934.4 // 100 mi
	if got := distanceInUnits(meters, "mi"); got != 100 {
		t.Fatalf("mi = %v, want 100", got)
	}
	if got := distanceInUnits(10000, "km"); got != 10 {
		t.Fatalf("km = %v, want 10", got)
	}
}
