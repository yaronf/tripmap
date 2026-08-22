package tripops

import (
	"context"
	"fmt"
	"math"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/routing"
)

const SummaryEstimateDrive = "OSRM road distance/time between waypoints (use instead of web search for drive estimates)"

// DriveWaypoint is one point on an estimateDrive request.
type DriveWaypoint struct {
	Place string  `json:"place,omitempty"` // place id from the trip catalog
	Lat   float64 `json:"lat,omitempty"`
	Lon   float64 `json:"lon,omitempty"`
	Title string  `json:"title,omitempty"` // optional label for the response
}

// DriveLeg is one OSRM leg between consecutive waypoints.
type DriveLeg struct {
	From             string  `json:"from"`
	To               string  `json:"to"`
	DistanceKm       float64 `json:"distance_km"`
	DurationMinutes  float64 `json:"duration_minutes"`
	DistanceMeters   float64 `json:"distance_meters"`
	DurationSeconds  float64 `json:"duration_seconds"`
	FallbackStraight bool    `json:"fallback_straight,omitempty"`
}

// DriveEstimate is the OSRM (or straight-line fallback) summary.
type DriveEstimate struct {
	Legs            []DriveLeg `json:"legs"`
	DistanceKm      float64    `json:"distance_km"`
	DurationMinutes float64    `json:"duration_minutes"`
	Provider        string     `json:"provider"` // osrm | mixed | straight
}

// EstimateDrive routes consecutive waypoints with OSRM (overview=false).
// Prefer place ids from getTripYAML; lat/lon may be used for candidates not yet in the trip.
func (s *Service) EstimateDrive(ctx context.Context, tripID string, points []DriveWaypoint) (DriveEstimate, error) {
	if len(points) < 2 {
		return DriveEstimate{}, badRequest(fmt.Errorf("need at least two waypoints"))
	}
	resolved, err := s.resolveDrivePoints(ctx, tripID, points)
	if err != nil {
		return DriveEstimate{}, err
	}

	var legs []DriveLeg
	var totalDist, totalDur float64
	osrmOK, straightOK := 0, 0
	for i := 0; i < len(resolved)-1; i++ {
		a, b := resolved[i], resolved[i+1]
		leg := DriveLeg{From: a.label, To: b.label}
		seg, segErr := routing.RouteOSRMBaseOverview(ctx, s.osrmBase,
			[]routing.Point{{Lat: a.lat, Lon: a.lon}, {Lat: b.lat, Lon: b.lon}}, "false")
		if segErr != nil || seg == nil {
			d := haversineMeters(a.lat, a.lon, b.lat, b.lon)
			leg.DistanceMeters = d
			leg.DurationSeconds = d / (80_000.0 / 3600.0) // ~80 km/h when OSRM fails
			leg.FallbackStraight = true
			straightOK++
		} else {
			leg.DistanceMeters = seg.DistanceMeters
			leg.DurationSeconds = seg.DurationSeconds
			osrmOK++
		}
		leg.DistanceKm = round1(leg.DistanceMeters / 1000)
		leg.DurationMinutes = round1(leg.DurationSeconds / 60)
		totalDist += leg.DistanceMeters
		totalDur += leg.DurationSeconds
		legs = append(legs, leg)
	}

	provider := "osrm"
	switch {
	case osrmOK == 0:
		provider = "straight"
	case straightOK > 0:
		provider = "mixed"
	}

	return DriveEstimate{
		Legs:            legs,
		DistanceKm:      round1(totalDist / 1000),
		DurationMinutes: round1(totalDur / 60),
		Provider:        provider,
	}, nil
}

type resolvedPoint struct {
	label string
	lat   float64
	lon   float64
}

func (s *Service) resolveDrivePoints(ctx context.Context, tripID string, points []DriveWaypoint) ([]resolvedPoint, error) {
	var trip *itinerary.Trip
	needPlaces := false
	for _, p := range points {
		if strings.TrimSpace(p.Place) != "" {
			needPlaces = true
			break
		}
	}
	if needPlaces {
		obj, err := s.store.GetYAML(ctx, tripID)
		if err != nil {
			return nil, notFound(err)
		}
		t, err := itinerary.ParseYAML(obj.Body)
		if err != nil {
			return nil, err
		}
		if err := itinerary.ResolvePlaces(&t); err != nil {
			return nil, badRequest(err)
		}
		trip = &t
	}

	out := make([]resolvedPoint, 0, len(points))
	for i, p := range points {
		placeID := strings.TrimSpace(p.Place)
		lat, lon := p.Lat, p.Lon
		label := strings.TrimSpace(p.Title)
		hasCoords := lat != 0 || lon != 0
		if placeID != "" {
			if trip == nil {
				return nil, badRequest(fmt.Errorf("waypoint %d: trip required to resolve place", i))
			}
			pl, ok := trip.Places[placeID]
			if !ok {
				return nil, badRequest(fmt.Errorf("waypoint %d: unknown place %q", i, placeID))
			}
			lat, lon = pl.Lat, pl.Lon
			if label == "" {
				label = pl.Title
				if label == "" {
					label = placeID
				}
			}
		} else if !hasCoords {
			return nil, badRequest(fmt.Errorf("waypoint %d: need place id or lat/lon", i))
		} else if label == "" {
			label = fmt.Sprintf("point_%d", i+1)
		}
		out = append(out, resolvedPoint{label: label, lat: lat, lon: lon})
	}
	return out, nil
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371000.0
	toRad := math.Pi / 180
	φ1, φ2 := lat1*toRad, lat2*toRad
	Δφ := (lat2 - lat1) * toRad
	Δλ := (lon2 - lon1) * toRad
	a := math.Sin(Δφ/2)*math.Sin(Δφ/2) + math.Cos(φ1)*math.Cos(φ2)*math.Sin(Δλ/2)*math.Sin(Δλ/2)
	return 2 * r * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
