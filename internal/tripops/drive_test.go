package tripops

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaronf/tripmap/internal/store"
)

func TestEstimateDrivePlaceIDs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/route/v1/driving/") {
			t.Fatalf("path %s", r.URL.Path)
		}
		if r.URL.Query().Get("overview") != "false" {
			t.Fatalf("want overview=false, got %q", r.URL.Query().Get("overview"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"routes": []map[string]any{{
				"distance": 42000.0,
				"duration": 2400.0,
				"geometry": map[string]any{"coordinates": [][]float64{}},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	st := store.NewMem()
	yaml := []byte(`schema_version: 2
trip: Test
places:
  a: {title: Alpha, lat: -41.0, lon: 174.0, type: town}
  b: {title: Bravo, lat: -41.3, lon: 174.8, type: town}
days:
  - title: Day 1
    route:
      - {place: a}
      - {place: b}
`)
	if _, err := st.PutYAML(context.Background(), "t1", yaml); err != nil {
		t.Fatal(err)
	}

	svc := New(Config{Store: st, RouteMode: "osrm", OSRMBaseURL: srv.URL})
	est, err := svc.EstimateDrive(context.Background(), "t1", []DriveWaypoint{
		{Place: "a"}, {Place: "b"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if est.Provider != "osrm" || len(est.Legs) != 1 {
		t.Fatalf("%+v", est)
	}
	if est.DistanceKm != 42 || est.DurationMinutes != 40 {
		t.Fatalf("totals %+v", est)
	}
	if est.Legs[0].From != "Alpha" || est.Legs[0].To != "Bravo" {
		t.Fatalf("labels %+v", est.Legs[0])
	}
}

func TestEstimateDriveLatLonAndUnknownPlace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"routes": []map[string]any{{
				"distance": 1000.0, "duration": 60.0,
				"geometry": map[string]any{"coordinates": [][]float64{}},
			}},
		})
	}))
	t.Cleanup(srv.Close)

	svc := New(Config{Store: store.NewMem(), OSRMBaseURL: srv.URL})
	est, err := svc.EstimateDrive(context.Background(), "missing", []DriveWaypoint{
		{Lat: -41.0, Lon: 174.0, Title: "A"},
		{Lat: -41.1, Lon: 174.1, Title: "B"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if est.DistanceKm != 1 || est.DurationMinutes != 1 {
		t.Fatalf("%+v", est)
	}

	_, err = svc.EstimateDrive(context.Background(), "missing", []DriveWaypoint{
		{Place: "nope"}, {Lat: 1, Lon: 2},
	})
	if err == nil || HTTPStatus(err) != http.StatusNotFound {
		// trip missing when resolving place
		t.Fatalf("want not found, got %v", err)
	}
}

func TestEstimateDriveNeedTwoPoints(t *testing.T) {
	svc := New(Config{Store: store.NewMem()})
	_, err := svc.EstimateDrive(context.Background(), "t", []DriveWaypoint{{Lat: 1, Lon: 2}})
	if err == nil || HTTPStatus(err) != http.StatusBadRequest {
		t.Fatalf("got %v", err)
	}
}
