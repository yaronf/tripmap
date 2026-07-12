package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func straightOpts(mode string) RouteOptions {
	return RouteOptions{Mode: mode, CoordPrecision: 6}
}

func testFolder(t *testing.T, d Day, routeMode string) Folder {
	t.Helper()
	f, err := buildFolder(context.Background(), d, straightOpts(routeMode), map[string]bool{})
	if err != nil {
		t.Fatalf("buildFolder: %v", err)
	}
	return f
}

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

	doc, err := buildDocument(context.Background(), trip, straightOpts("straight"))
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

func countPoints(doc Document) int {
	n := 0
	for _, f := range doc.Folders {
		for _, pm := range f.Placemarks {
			if pm.Point != nil {
				n++
			}
		}
	}
	return n
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

	f := testFolder(t, d, "straight")

	names := placemarkNames(f)
	want := []string{"A", "B", "Route"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("placemarks = %v, want %v", names, want)
	}

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

	f := testFolder(t, d, "straight")

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

	f := testFolder(t, d, "straight")

	if got := placemarkNames(f); strings.Join(got, ",") != "A,B" {
		t.Fatalf("placemarks = %v, want [A B] with no route", got)
	}
}

func TestHikeDayTrailSegmentIsStraight(t *testing.T) {
	d := Day{
		Day:   1,
		Title: "hike",
		Hike:  true,
		Route: []Stop{
			{Name: "Trailhead", Type: "trailhead", Lat: 1, Lon: 1},
			{Name: "Hut", Type: "hut", Lat: 2, Lon: 2},
		},
	}

	f := testFolder(t, d, "osrm")

	route := f.Placemarks[len(f.Placemarks)-1]
	if route.StyleURL != "#hikeLine" {
		t.Fatalf("route styleUrl = %q, want #hikeLine", route.StyleURL)
	}
	want := "1.000000,1.000000,0\n2.000000,2.000000,0"
	if route.Line == nil || route.Line.Coordinates != want {
		t.Fatalf("hike line = %+v, want straight coords %q", route.Line, want)
	}
}

func TestFerryDaySplitsFerryAndDriveSegments(t *testing.T) {
	d := Day{
		Day:   7,
		Title: "ferry and drive",
		Ferry: true,
		Route: []Stop{
			{Name: "Wellington", Type: "ferry_terminal", Lat: -41.2642, Lon: 174.7870},
			{Name: "Picton", Type: "ferry_terminal", Lat: -41.2855, Lon: 174.0050},
			{Name: "Nelson", Type: "overnight", Lat: -41.2706, Lon: 173.2840},
		},
	}

	f := testFolder(t, d, "straight")

	if len(f.Placemarks) < 4 {
		t.Fatalf("placemarks = %d, want ferry + drive lines after map points", len(f.Placemarks))
	}
	ferry := f.Placemarks[len(f.Placemarks)-2]
	drive := f.Placemarks[len(f.Placemarks)-1]
	if ferry.StyleURL != "#ferryLine" {
		t.Fatalf("ferry styleUrl = %q, want #ferryLine", ferry.StyleURL)
	}
	if drive.StyleURL != "#driveLine" {
		t.Fatalf("drive styleUrl = %q, want #driveLine", drive.StyleURL)
	}
	wantFerry := "174.787000,-41.264200,0\n174.005000,-41.285500,0"
	if ferry.Line == nil || ferry.Line.Coordinates != wantFerry {
		t.Fatalf("ferry line = %+v, want %q", ferry.Line, wantFerry)
	}
	wantDrive := "174.005000,-41.285500,0\n173.284000,-41.270600,0"
	if drive.Line == nil || drive.Line.Coordinates != wantDrive {
		t.Fatalf("drive line = %+v, want %q", drive.Line, wantDrive)
	}
}

func TestHikeDayAddsDriveFromLodgingInStops(t *testing.T) {
	d := Day{
		Day:   5,
		Title: "crossing",
		Hike:  true,
		Route: []Stop{
			{Name: "Mangatepopo", Type: "trailhead", Lat: 1, Lon: 1},
			{Name: "Ketetahi", Type: "trailhead", Lat: 2, Lon: 2},
		},
		Stops: []Stop{
			{Name: "National Park", Type: "overnight", Lat: 0, Lon: 0},
		},
	}

	f := testFolder(t, d, "straight")
	names := placemarkNames(f)
	if names[0] != "National Park" {
		t.Fatalf("placemarks = %v, want lodging first", names)
	}
	if len(f.Placemarks) < 3 {
		t.Fatalf("placemarks = %v, want drive + hike lines", names)
	}
	if f.Placemarks[len(f.Placemarks)-2].StyleURL != "#driveLine" {
		t.Fatalf("expected drive line before hike segment")
	}
	if f.Placemarks[len(f.Placemarks)-1].StyleURL != "#hikeLine" {
		t.Fatalf("expected hike line for trail segment")
	}
}

func TestGlobalPlacemarkDedup(t *testing.T) {
	wanaka := Stop{Name: "Wanaka", Type: "overnight", Lat: -44.6966, Lon: 169.1362}
	trip := Trip{
		Trip: "dedup",
		Days: []Day{
			{
				Day: 12, Title: "arrive",
				Route: []Stop{wanaka, {Name: "Franz Josef", Type: "overnight", Lat: -43.3881, Lon: 170.1836}},
			},
			{
				Day: 13, Title: "stay",
				Route: []Stop{wanaka, {Name: "Roys Peak", Type: "trailhead", Lat: -44.6735, Lon: 169.0718}},
				Hike:  true,
			},
			{
				Day: 14, Title: "leave",
				Route: []Stop{wanaka, {Name: "Te Anau", Type: "overnight", Lat: -45.4145, Lon: 167.7180}},
			},
		},
	}

	doc, err := buildDocument(context.Background(), trip, straightOpts("straight"))
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	if count := countPoints(doc); count != 4 {
		t.Fatalf("point placemarks = %d, want 4 unique locations (Wanaka once)", count)
	}
}

func TestAirportStopEmitsStyle(t *testing.T) {
	trip := Trip{
		Trip: "airport",
		Days: []Day{{
			Day:   1,
			Title: "arrive",
			Route: []Stop{
				{Name: "Schiphol", Type: "airport", Lat: 52.3086, Lon: 4.7639},
				{Name: "Amsterdam", Type: "overnight", Lat: 52.3676, Lon: 4.9041},
			},
		}},
	}

	doc, err := buildDocument(context.Background(), trip, straightOpts("straight"))
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	var hasAirport, hasDrive bool
	for _, s := range doc.Styles {
		switch s.ID {
		case "airport":
			hasAirport = true
		case "driveLine":
			hasDrive = true
		}
	}
	if !hasAirport || !hasDrive {
		t.Fatalf("styles = %+v, want airport and driveLine", doc.Styles)
	}
	pm := doc.Folders[0].Placemarks[0]
	if pm.StyleURL != "#airport" {
		t.Fatalf("airport styleUrl = %q, want #airport", pm.StyleURL)
	}
}

func TestMyMapsFlattenRemovesFolders(t *testing.T) {
	trip := Trip{
		Trip: "flat",
		Days: []Day{
			{Day: 1, Title: "empty"},
			{
				Day: 2, Title: "drive", Notes: "note",
				Route: []Stop{
					{Name: "A", Type: "overnight", Lat: 1, Lon: 1},
					{Name: "B", Type: "overnight", Lat: 2, Lon: 2},
				},
			},
		},
	}

	doc, err := buildDocument(context.Background(), trip, RouteOptions{
		Mode: "straight", CoordPrecision: 6, Flatten: true,
	})
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	if len(doc.Folders) != 0 {
		t.Fatalf("folders = %d, want 0", len(doc.Folders))
	}
	if len(doc.Placemarks) != 3 {
		t.Fatalf("placemarks = %d, want 3 (A, B, Route)", len(doc.Placemarks))
	}
	if doc.Placemarks[0].Name != "Day 2 - drive: A" {
		t.Fatalf("first name = %q", doc.Placemarks[0].Name)
	}
}

func TestSplitLongLinePlacemark(t *testing.T) {
	var coords []string
	for i := 0; i < 600; i++ {
		coords = append(coords, fmt.Sprintf("%f,%f,0", float64(i), 0.0))
	}
	pm := Placemark{
		Name:     "Route",
		StyleURL: "#driveLine",
		Line:     &Line{Coordinates: strings.Join(coords, "\n")},
	}
	parts := splitLongLinePlacemark(pm, 499)
	if len(parts) != 2 {
		t.Fatalf("parts = %d, want 2", len(parts))
	}
	if parts[1].Name != "Route (2)" {
		t.Fatalf("second name = %q", parts[1].Name)
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

	doc, err := buildDocument(context.Background(), trip, straightOpts("straight"))
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	if len(doc.Styles) != 1 || doc.Styles[0].ID != "overnight" {
		t.Fatalf("styles = %+v, want single overnight style", doc.Styles)
	}
}

func TestUntypedItineraryEmitsDriveLineStyle(t *testing.T) {
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

	doc, err := buildDocument(context.Background(), trip, straightOpts("straight"))
	if err != nil {
		t.Fatalf("buildDocument: %v", err)
	}
	if len(doc.Styles) != 1 || doc.Styles[0].ID != "driveLine" {
		t.Fatalf("styles = %+v, want single driveLine style", doc.Styles)
	}
}
