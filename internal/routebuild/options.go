package routebuild

import (
	"fmt"

	"github.com/yaronf/tripmap/internal/itinerary"
)

// RouteOptions controls road routing and coordinate detail.
type RouteOptions struct {
	Mode           string
	SimplifyMeters float64
	CoordPrecision int
	Flatten        bool   // flat placemarks for Google My Maps (no Folders)
	Units          string // km (default) or mi — distance display for PWA bundles
	OSRMBase       string // optional OSRM origin; empty uses the public server
}

// Segment is one styled polyline (drive, hike, ferry, or unrouted fallback).
type Segment struct {
	Name            string
	Style           string // driveLine, hikeLine, ferryLine, fallbackLine
	Geometry        [][]float64
	DistanceMeters  float64
	DurationSeconds float64 // set for OSRM drive segments; 0 for straight/fallback
}

type routedGeom struct {
	Geometry        [][]float64
	DistanceMeters  float64
	DurationSeconds float64
	Fallback        bool
}

// StopKey is a stable location key for de-duplication.
func StopKey(s itinerary.Stop) string {
	return fmt.Sprintf("%.6f|%.6f", s.Lat, s.Lon)
}

func sameStop(a, b itinerary.Stop) bool {
	return StopKey(a) == StopKey(b)
}
