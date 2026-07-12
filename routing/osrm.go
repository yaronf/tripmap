package routing

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var defaultHTTPClient = &http.Client{Timeout: 30 * time.Second}

const defaultOSRMBase = "https://router.project-osrm.org"

type Point struct {
	Lat float64
	Lon float64
}

type Route struct {
	DistanceMeters  float64
	DurationSeconds float64
	Geometry        [][]float64 // [lon,lat]
}

func RouteOSRM(ctx context.Context, pts []Point) (*Route, error) {
	return routeOSRM(ctx, defaultOSRMBase, pts, defaultHTTPClient, "full")
}

// RouteOSRMOverview requests a route from OSRM. overview is one of full,
// simplified, or false (see OSRM API docs).
func RouteOSRMOverview(ctx context.Context, pts []Point, overview string) (*Route, error) {
	return routeOSRM(ctx, defaultOSRMBase, pts, defaultHTTPClient, overview)
}

func routeOSRM(ctx context.Context, baseURL string, pts []Point, client *http.Client, overview string) (*Route, error) {
	if len(pts) < 2 {
		return nil, fmt.Errorf("need at least two points")
	}

	var b strings.Builder
	for i, p := range pts {
		if i > 0 {
			b.WriteByte(';')
		}
		fmt.Fprintf(&b, "%f,%f", p.Lon, p.Lat)
	}

	if overview == "" {
		overview = "full"
	}

	url := fmt.Sprintf(
		"%s/route/v1/driving/%s?overview=%s&geometries=geojson",
		strings.TrimRight(baseURL, "/"),
		b.String(),
		overview,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("osrm request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var r struct {
		Routes []struct {
			Distance float64 `json:"distance"`
			Duration float64 `json:"duration"`
			Geometry struct {
				Coordinates [][]float64 `json:"coordinates"`
			} `json:"geometry"`
		} `json:"routes"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, err
	}
	if len(r.Routes) == 0 {
		return nil, fmt.Errorf("no route returned")
	}

	x := r.Routes[0]
	return &Route{
		DistanceMeters:  x.Distance,
		DurationSeconds: x.Duration,
		Geometry:        x.Geometry.Coordinates,
	}, nil
}
