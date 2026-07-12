package routing

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRouteOSRM(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/route/v1/driving/1.000000,2.000000;3.000000,4.000000" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"routes": []map[string]any{
				{
					"distance": 1000.0,
					"duration": 120.0,
					"geometry": map[string]any{
						"coordinates": [][]float64{{1, 2}, {3, 4}},
					},
				},
			},
		})
	}))
	defer srv.Close()

	route, err := routeOSRM(context.Background(), srv.URL, []Point{
		{Lat: 2, Lon: 1},
		{Lat: 4, Lon: 3},
	}, srv.Client())
	if err != nil {
		t.Fatalf("routeOSRM: %v", err)
	}
	if route.DistanceMeters != 1000 {
		t.Fatalf("distance = %v, want 1000", route.DistanceMeters)
	}
	if route.DurationSeconds != 120 {
		t.Fatalf("duration = %v, want 120", route.DurationSeconds)
	}
	if len(route.Geometry) != 2 {
		t.Fatalf("geometry len = %d, want 2", len(route.Geometry))
	}
}

func TestRouteOSRMStatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	_, err := routeOSRM(context.Background(), srv.URL, []Point{
		{Lat: 2, Lon: 1},
		{Lat: 4, Lon: 3},
	}, srv.Client())
	if err == nil {
		t.Fatal("expected error for non-200 response")
	}
}

func TestRouteOSRMNeedTwoPoints(t *testing.T) {
	_, err := RouteOSRM(context.Background(), []Point{{Lat: 1, Lon: 2}})
	if err == nil {
		t.Fatal("expected error for single point")
	}
}
