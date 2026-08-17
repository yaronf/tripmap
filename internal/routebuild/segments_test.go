package routebuild

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaronf/tripmap/internal/itinerary"
)

func osrmOK(coords [][]float64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"routes": []map[string]any{{
				"distance": 1000.0,
				"duration": 120.0,
				"geometry": map[string]any{"coordinates": coords},
			}},
		})
	}
}

func osrmNoRoute(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(map[string]any{"routes": []any{}})
}

func TestBuildRouteSegmentsKeepsSuccessfulLegsOnFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail lodge→trailhead (unroutable trail); all other requests succeed.
		if strings.Contains(r.URL.Path, "10.000000,52.000000;11.000000,52.100000") {
			osrmNoRoute(w, r)
			return
		}
		osrmOK([][]float64{{12, 52}, {13, 52.2}})(w, r)
	}))
	t.Cleanup(srv.Close)

	d := itinerary.Day{
		Day:  1,
		Hike: true,
		Route: []itinerary.Stop{
			{Name: "Lodge", Type: "overnight", Lat: 52.0, Lon: 10.0},
			{Name: "Trailhead", Type: "trailhead", Lat: 52.1, Lon: 11.0},
			{Name: "Hut", Type: "hut", Lat: 52.2, Lon: 12.0},
		},
	}
	opts := RouteOptions{Mode: "osrm", CoordPrecision: 6, OSRMBase: srv.URL}

	segs, err := BuildRouteSegments(context.Background(), d, d.Route, opts)
	if err != nil {
		t.Fatalf("BuildRouteSegments: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("len(segs) = %d, want 2", len(segs))
	}
	if segs[0].Style != "fallbackLine" {
		t.Fatalf("approach style = %q, want fallbackLine", segs[0].Style)
	}
	wantFallback := [][]float64{{10, 52}, {11, 52.1}}
	if got := fmtCoords(segs[0].Geometry); got != fmtCoords(wantFallback) {
		t.Fatalf("fallback geom = %v, want %v", segs[0].Geometry, wantFallback)
	}
	if segs[1].Style != "hikeLine" {
		t.Fatalf("trail style = %q, want hikeLine", segs[1].Style)
	}
}

func TestBuildRouteSegmentsDriveDayPairwiseFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := strings.Count(r.URL.Path, ";")
		if n >= 2 {
			osrmNoRoute(w, r)
			return
		}
		if strings.Contains(r.URL.Path, "1.000000,1.000000;2.000000,2.000000") {
			osrmOK([][]float64{{1, 1}, {1.5, 1.5}, {2, 2}})(w, r)
			return
		}
		osrmNoRoute(w, r)
	}))
	t.Cleanup(srv.Close)

	pts := []itinerary.Stop{
		{Name: "A", Type: "overnight", Lat: 1, Lon: 1},
		{Name: "B", Type: "via", Lat: 2, Lon: 2},
		{Name: "C", Type: "overnight", Lat: 3, Lon: 3},
	}
	opts := RouteOptions{Mode: "osrm", CoordPrecision: 6, OSRMBase: srv.URL}

	segs, err := BuildRouteSegments(context.Background(), itinerary.Day{Day: 1}, pts, opts)
	if err != nil {
		t.Fatalf("BuildRouteSegments: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("len(segs) = %d, want 2", len(segs))
	}
	if segs[0].Style != "driveLine" {
		t.Fatalf("first style = %q, want driveLine", segs[0].Style)
	}
	if segs[1].Style != "fallbackLine" {
		t.Fatalf("second style = %q, want fallbackLine", segs[1].Style)
	}
	if got := fmtCoords(segs[1].Geometry); got != fmtCoords([][]float64{{2, 2}, {3, 3}}) {
		t.Fatalf("fallback geom = %v", segs[1].Geometry)
	}
}

func TestBuildRouteSegmentsPreservesSuccessfulDrive(t *testing.T) {
	srv := httptest.NewServer(osrmOK([][]float64{{1, 1}, {2, 2}, {3, 3}}))
	t.Cleanup(srv.Close)

	pts := []itinerary.Stop{
		{Name: "A", Type: "overnight", Lat: 1, Lon: 1},
		{Name: "B", Type: "overnight", Lat: 3, Lon: 3},
	}
	opts := RouteOptions{Mode: "osrm", CoordPrecision: 6, OSRMBase: srv.URL}
	segs, err := BuildRouteSegments(context.Background(), itinerary.Day{Day: 1}, pts, opts)
	if err != nil {
		t.Fatalf("BuildRouteSegments: %v", err)
	}
	if len(segs) != 1 || segs[0].Style != "driveLine" {
		t.Fatalf("segs = %+v, want one driveLine", segs)
	}
}

func TestBuildRouteSegmentsFerryKeepsFerryAndFallbacksDrive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		osrmNoRoute(w, r)
	}))
	t.Cleanup(srv.Close)

	pts := []itinerary.Stop{
		{Name: "Town", Type: "overnight", Lat: 1, Lon: 1},
		{Name: "Dock A", Type: "ferry_terminal", Lat: 2, Lon: 2},
		{Name: "Dock B", Type: "ferry_terminal", Lat: 3, Lon: 3},
	}
	opts := RouteOptions{Mode: "osrm", CoordPrecision: 6, OSRMBase: srv.URL}
	segs, err := BuildRouteSegments(context.Background(), itinerary.Day{Day: 1, Ferry: true}, pts, opts)
	if err != nil {
		t.Fatalf("BuildRouteSegments: %v", err)
	}
	if len(segs) != 2 {
		t.Fatalf("len(segs) = %d, want 2", len(segs))
	}
	if segs[0].Style != "fallbackLine" {
		t.Fatalf("drive-to-dock style = %q, want fallbackLine", segs[0].Style)
	}
	if segs[1].Style != "ferryLine" {
		t.Fatalf("crossing style = %q, want ferryLine", segs[1].Style)
	}
}

func fmtCoords(g [][]float64) string {
	b, _ := json.Marshal(g)
	return string(b)
}
