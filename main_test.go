package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildKMLStraightGolden(t *testing.T) {
	input := filepath.Join("testdata", "itinerary.yaml")
	golden := filepath.Join("testdata", "trip_straight.golden.kml")

	b, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("read input: %v", err)
	}

	var trip Trip
	if err := yaml.Unmarshal(b, &trip); err != nil {
		t.Fatalf("parse yaml: %v", err)
	}

	doc, err := buildDocument(context.Background(), trip, "straight")
	if err != nil {
		t.Fatalf("build document: %v", err)
	}

	got, err := marshalKML(doc)
	if err != nil {
		t.Fatalf("marshal kml: %v", err)
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("kml output differs from golden file %s", golden)
	}
}

func TestRunInvalidRouteMode(t *testing.T) {
	err := run([]string{
		"-input", filepath.Join("testdata", "itinerary.yaml"),
		"-output", t.TempDir() + "/out.kml",
		"-route", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid route mode")
	}
}

func placemarkNames(f Folder) []string {
	names := make([]string, len(f.Placemarks))
	for i, pm := range f.Placemarks {
		names[i] = pm.Name
	}
	return names
}

func TestViaPointsAreRoutedButNotMapped(t *testing.T) {
	d := Day{
		Day:   1,
		Title: "with via",
		Route: []Stop{
			{Name: "A", Type: "overnight", Lat: 1, Lon: 1},
			{Name: "V", Type: "via", Lat: 2, Lon: 2},
			{Name: "B", Type: "overnight", Lat: 3, Lon: 3},
		},
	}

	f, err := buildFolder(context.Background(), d, "straight")
	if err != nil {
		t.Fatalf("buildFolder: %v", err)
	}

	// Placemarks: A, B, Route (no V).
	names := placemarkNames(f)
	want := []string{"A", "B", "Route"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("placemarks = %v, want %v", names, want)
	}

	// The via point must still shape the route line.
	route := f.Placemarks[len(f.Placemarks)-1]
	if route.Line == nil || !strings.Contains(route.Line.Coordinates, "2.000000,2.000000,0") {
		t.Fatalf("via point missing from route line: %+v", route.Line)
	}
}

func TestAttractionsAreMappedButNotRouted(t *testing.T) {
	d := Day{
		Day:   1,
		Title: "attraction",
		Stops: []Stop{
			{Name: "Town", Type: "attraction", Lat: 1, Lon: 1},
		},
	}

	f, err := buildFolder(context.Background(), d, "straight")
	if err != nil {
		t.Fatalf("buildFolder: %v", err)
	}

	if got := placemarkNames(f); len(got) != 1 || got[0] != "Town" {
		t.Fatalf("placemarks = %v, want [Town] with no route", got)
	}
}

func TestStopsDoNotImplicitlyCreateRoute(t *testing.T) {
	d := Day{
		Day:   1,
		Title: "stops only",
		Stops: []Stop{
			{Name: "A", Type: "overnight", Lat: 1, Lon: 1},
			{Name: "B", Type: "overnight", Lat: 2, Lon: 2},
		},
	}

	f, err := buildFolder(context.Background(), d, "straight")
	if err != nil {
		t.Fatalf("buildFolder: %v", err)
	}

	if got := placemarkNames(f); strings.Join(got, ",") != "A,B" {
		t.Fatalf("placemarks = %v, want [A B] with no route", got)
	}
}

func TestHikeDayForcesStraightLine(t *testing.T) {
	d := Day{
		Day:   1,
		Title: "hike",
		Hike:  true,
		Route: []Stop{
			{Name: "Trailhead", Type: "trailhead", Lat: 1, Lon: 1},
			{Name: "Hut", Type: "trailhead", Lat: 2, Lon: 2},
		},
	}

	// osrm mode requested, but hike days must not hit the network.
	f, err := buildFolder(context.Background(), d, "osrm")
	if err != nil {
		t.Fatalf("buildFolder: %v", err)
	}

	route := f.Placemarks[len(f.Placemarks)-1]
	if route.StyleURL != "#hikeLine" {
		t.Fatalf("route styleUrl = %q, want #hikeLine", route.StyleURL)
	}
	want := "1.000000,1.000000,0 2.000000,2.000000,0"
	if route.Line == nil || route.Line.Coordinates != want {
		t.Fatalf("hike line = %+v, want straight coords %q", route.Line, want)
	}
}

func TestTypedStopsEmitStyles(t *testing.T) {
	trip := Trip{
		Trip: "styled",
		Days: []Day{{
			Day:   1,
			Title: "d",
			Stops: []Stop{{Name: "Lodge", Type: "overnight", Lat: 1, Lon: 1}},
		}},
	}

	doc, err := buildDocument(context.Background(), trip, "straight")
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	if len(doc.Styles) != 1 || doc.Styles[0].ID != "overnight" {
		t.Fatalf("styles = %+v, want single overnight style", doc.Styles)
	}
}

func TestUntypedItineraryEmitsNoStyles(t *testing.T) {
	trip := Trip{
		Trip: "plain",
		Days: []Day{{
			Day:   1,
			Title: "d",
			Route: []Stop{
				{Name: "A", Lat: 1, Lon: 1},
				{Name: "B", Lat: 2, Lon: 2},
			},
		}},
	}

	doc, err := buildDocument(context.Background(), trip, "straight")
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	if len(doc.Styles) != 0 {
		t.Fatalf("styles = %+v, want none for untyped itinerary", doc.Styles)
	}
}
