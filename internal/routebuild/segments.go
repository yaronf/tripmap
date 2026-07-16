package routebuild

import (
	"context"
	"fmt"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/routing"
)

// BuildRouteSegments returns styled polylines for a day's route.
func BuildRouteSegments(ctx context.Context, d itinerary.Day, pts []itinerary.Stop, opts RouteOptions) ([]Segment, error) {
	if d.Ferry {
		return buildSegmentGeometries(ctx, pts, opts, func(a, b itinerary.Stop) (mode, style, name string) {
			if isFerrySegment(a, b) {
				return "straight", "ferryLine", "Ferry"
			}
			return opts.Mode, "driveLine", "Drive"
		})
	}

	if !d.Hike {
		rg, err := routeGeometry(ctx, pts, opts.Mode, opts)
		if err != nil {
			return nil, err
		}
		return []Segment{{
			Name: "Route", Style: "driveLine", Geometry: rg.Geometry,
			DistanceMeters: rg.DistanceMeters, DurationSeconds: rg.DurationSeconds,
		}}, nil
	}

	return buildSegmentGeometries(ctx, pts, opts, func(a, b itinerary.Stop) (mode, style, name string) {
		if isDrivingSegment(a, b) {
			return opts.Mode, "driveLine", "Drive"
		}
		return "straight", "hikeLine", "Hike"
	})
}

func buildSegmentGeometries(ctx context.Context, pts []itinerary.Stop, opts RouteOptions, classify func(a, b itinerary.Stop) (mode, style, name string)) ([]Segment, error) {
	var segs []Segment
	for i := 0; i < len(pts)-1; i++ {
		seg := []itinerary.Stop{pts[i], pts[i+1]}
		mode, style, name := classify(seg[0], seg[1])
		rg, err := routeGeometry(ctx, seg, mode, opts)
		if err != nil {
			return nil, err
		}
		segs = append(segs, Segment{
			Name: name, Style: style, Geometry: rg.Geometry,
			DistanceMeters: rg.DistanceMeters, DurationSeconds: rg.DurationSeconds,
		})
	}
	if len(segs) == 1 {
		segs[0].Name = "Route"
	}
	return segs, nil
}

func routeGeometry(ctx context.Context, stops []itinerary.Stop, mode string, opts RouteOptions) (routedGeom, error) {
	switch mode {
	case "straight":
		out := make([][]float64, len(stops))
		for i, s := range stops {
			out[i] = []float64{s.Lon, s.Lat}
		}
		return routedGeom{Geometry: out, DistanceMeters: routing.PathLengthMeters(out)}, nil
	case "osrm":
		pts := make([]routing.Point, len(stops))
		for i, s := range stops {
			pts[i] = routing.Point{Lat: s.Lat, Lon: s.Lon}
		}
		route, err := routing.RouteOSRM(ctx, pts)
		if err != nil {
			return routedGeom{}, err
		}
		return routedGeom{
			Geometry:        routing.SimplifyGeometry(route.Geometry, opts.SimplifyMeters),
			DistanceMeters:  route.DistanceMeters,
			DurationSeconds: route.DurationSeconds,
		}, nil
	default:
		return routedGeom{}, fmt.Errorf("unknown route mode %q (use straight or osrm)", mode)
	}
}

// RouteGeometryCoords returns KML coordinate string for a polyline.
func RouteGeometryCoords(ctx context.Context, stops []itinerary.Stop, mode string, opts RouteOptions) (string, error) {
	rg, err := routeGeometry(ctx, stops, mode, opts)
	if err != nil {
		return "", err
	}
	return GeometryToKMLCoords(rg.Geometry, opts.CoordPrecision), nil
}

// GeometryToKMLCoords formats lon,lat,0 lines for KML.
func GeometryToKMLCoords(geometry [][]float64, precision int) string {
	parts := make([]string, len(geometry))
	for i, pt := range geometry {
		parts[i] = FormatCoords(pt[0], pt[1], precision)
	}
	return strings.Join(parts, "\n")
}

// FormatCoords formats a single point for KML.
func FormatCoords(lon, lat float64, precision int) string {
	if precision <= 0 {
		precision = 6
	}
	format := fmt.Sprintf("%%.%df,%%.%df,0", precision, precision)
	return fmt.Sprintf(format, lon, lat)
}
