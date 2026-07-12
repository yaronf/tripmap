package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/yaronf/tripmap/routing"
)

// Stop is a single point in a day. Type controls whether it appears on the map,
// on the route line, or both:
//
//	""              generic waypoint: map placemark + route point (default)
//	overnight       lodging: map placemark + route endpoint
//	via             route-shaping only: on the line, no placemark
//	attraction      map placemark only, not on the route
//	viewpoint       map placemark only, not on the route
//	trailhead       map placemark only (use an explicit route: for hike lines)
//	ferry_terminal  map placemark + route point (draw with ferry styling)
type Stop struct {
	Name     string `yaml:"name"`
	Lat, Lon float64
	Type     string `yaml:"type,omitempty"`
	Notes    string `yaml:"notes,omitempty"`
}

// Day is one day of the trip. Route explicitly defines the line, while Stops
// defines additional map placemarks. Hike/Ferry routes are drawn as straight
// lines.
type Day struct {
	Day   int    `yaml:"day"`
	Title string `yaml:"title"`
	Route []Stop `yaml:"route,omitempty"`
	Stops []Stop `yaml:"stops,omitempty"`
	Notes string `yaml:"notes,omitempty"`
	Hike  bool   `yaml:"hike,omitempty"`
	Ferry bool   `yaml:"ferry,omitempty"`
}

type Trip struct {
	Trip        string `yaml:"trip"`
	Description string `yaml:"description,omitempty"`
	Days        []Day  `yaml:"days"`
}

type KML struct {
	XMLName xml.Name `xml:"kml"`
	Xmlns   string   `xml:"xmlns,attr"`
	Doc     Document `xml:"Document"`
}

type Document struct {
	Name        string   `xml:"name"`
	Description string   `xml:"description,omitempty"`
	Styles      []Style  `xml:"Style,omitempty"`
	Folders     []Folder `xml:"Folder"`
}

type Style struct {
	ID        string     `xml:"id,attr"`
	IconStyle *IconStyle `xml:"IconStyle,omitempty"`
	LineStyle *LineStyle `xml:"LineStyle,omitempty"`
}

type IconStyle struct {
	Color string `xml:"color,omitempty"`
	Icon  Icon   `xml:"Icon"`
}

type Icon struct {
	Href string `xml:"href"`
}

type LineStyle struct {
	Color string  `xml:"color,omitempty"`
	Width float64 `xml:"width,omitempty"`
}

type Folder struct {
	Name        string      `xml:"name"`
	Description string      `xml:"description,omitempty"`
	Placemarks  []Placemark `xml:"Placemark"`
}

type Placemark struct {
	Name        string `xml:"name"`
	Description string `xml:"description,omitempty"`
	StyleURL    string `xml:"styleUrl,omitempty"`
	Point       *Point `xml:"Point,omitempty"`
	Line        *Line  `xml:"LineString,omitempty"`
}

type Point struct {
	Coordinates string `xml:"coordinates"`
}

type Line struct {
	Tess         int    `xml:"tessellate"`
	AltitudeMode string `xml:"altitudeMode,omitempty"`
	Coordinates  string `xml:"coordinates"`
}

// iconStyles defines the marker style for each placemark type, in output order.
// KML colors are aabbggrr hex.
var iconStyles = []struct {
	Type, Color, Href string
}{
	{"overnight", "ff0000ff", "http://maps.google.com/mapfiles/kml/shapes/lodging.png"},
	{"attraction", "ff00aaff", "http://maps.google.com/mapfiles/kml/shapes/star.png"},
	{"viewpoint", "ffff8800", "http://maps.google.com/mapfiles/kml/shapes/camera.png"},
	{"trailhead", "ff00aa00", "http://maps.google.com/mapfiles/kml/shapes/hiker.png"},
	{"ferry_terminal", "ffaa5500", "http://maps.google.com/mapfiles/kml/shapes/ferry.png"},
}

// lineStyles defines styles for non-driving route lines, in output order.
var lineStyles = []struct {
	ID, Color string
	Width     float64
}{
	{"driveLine", "ffff0000", 4}, // blue in KML aabbggrr
	{"ferryLine", "ffff8000", 4},
	{"hikeLine", "ff00aa00", 4},
}

func styleForType(t string) string {
	for _, s := range iconStyles {
		if s.Type == t {
			return t
		}
	}
	return ""
}

// routePoints returns the ordered points that shape the day's route line.
func routePoints(d Day) []Stop {
	return d.Route
}

// mapPoints returns the ordered placemarks for a day, drawn from both Stops and
// Route, excluding via points and de-duplicating repeated coordinates.
func mapPoints(d Day) []Stop {
	var pts []Stop
	seen := map[string]bool{}
	add := func(s Stop) {
		if s.Type == "via" {
			return
		}
		key := fmt.Sprintf("%s|%f|%f", s.Name, s.Lat, s.Lon)
		if seen[key] {
			return
		}
		seen[key] = true
		pts = append(pts, s)
	}
	for _, s := range d.Stops {
		add(s)
	}
	for _, s := range d.Route {
		add(s)
	}
	return pts
}

func buildDocument(ctx context.Context, t Trip, routeMode string) (Document, error) {
	doc := Document{Name: t.Trip, Description: t.Description}
	for _, d := range t.Days {
		f, err := buildFolder(ctx, d, routeMode)
		if err != nil {
			return Document{}, fmt.Errorf("day %d: %w", d.Day, err)
		}
		doc.Folders = append(doc.Folders, f)
	}
	doc.Styles = usedStyles(doc.Folders)
	return doc, nil
}

func buildFolder(ctx context.Context, d Day, routeMode string) (Folder, error) {
	f := Folder{Name: fmt.Sprintf("Day %d - %s", d.Day, d.Title), Description: d.Notes}

	for _, s := range mapPoints(d) {
		pm := Placemark{
			Name:        s.Name,
			Description: s.Notes,
			Point:       &Point{Coordinates: stopCoords(s)},
		}
		if id := styleForType(s.Type); id != "" {
			pm.StyleURL = "#" + id
		}
		f.Placemarks = append(f.Placemarks, pm)
	}

	rp := routePoints(d)
	if len(rp) >= 2 {
		mode := routeMode
		if d.Hike || d.Ferry {
			mode = "straight"
		}
		coords, err := routeCoords(ctx, rp, mode)
		if err != nil {
			return Folder{}, err
		}
		line := Placemark{
			Name: "Route",
			Line: &Line{
				Tess:         1,
				AltitudeMode: "clampToGround",
				Coordinates:  coords,
			},
		}
		switch {
		case d.Ferry:
			line.StyleURL = "#ferryLine"
		case d.Hike:
			line.StyleURL = "#hikeLine"
		default:
			line.StyleURL = "#driveLine"
		}
		f.Placemarks = append(f.Placemarks, line)
	}
	return f, nil
}

// usedStyles returns the Style definitions referenced by any placemark, in a
// deterministic order. Styles that are never referenced are omitted so simple
// itineraries produce no <Style> elements.
func usedStyles(folders []Folder) []Style {
	used := map[string]bool{}
	for _, f := range folders {
		for _, pm := range f.Placemarks {
			if pm.StyleURL != "" {
				used[strings.TrimPrefix(pm.StyleURL, "#")] = true
			}
		}
	}

	var styles []Style
	for _, s := range iconStyles {
		if used[s.Type] {
			styles = append(styles, Style{
				ID:        s.Type,
				IconStyle: &IconStyle{Color: s.Color, Icon: Icon{Href: s.Href}},
			})
		}
	}
	for _, s := range lineStyles {
		if used[s.ID] {
			styles = append(styles, Style{
				ID:        s.ID,
				LineStyle: &LineStyle{Color: s.Color, Width: s.Width},
			})
		}
	}
	return styles
}

func routeCoords(ctx context.Context, stops []Stop, mode string) (string, error) {
	switch mode {
	case "straight":
		return straightLineCoords(stops), nil
	case "osrm":
		pts := make([]routing.Point, len(stops))
		for i, s := range stops {
			pts[i] = routing.Point{Lat: s.Lat, Lon: s.Lon}
		}
		route, err := routing.RouteOSRM(ctx, pts)
		if err != nil {
			return "", err
		}
		return geometryToKMLCoords(route.Geometry), nil
	default:
		return "", fmt.Errorf("unknown route mode %q (use straight or osrm)", mode)
	}
}

func stopCoords(s Stop) string {
	return fmt.Sprintf("%f,%f,0", s.Lon, s.Lat)
}

func straightLineCoords(stops []Stop) string {
	parts := make([]string, len(stops))
	for i, s := range stops {
		parts[i] = stopCoords(s)
	}
	return strings.Join(parts, "\n")
}

func geometryToKMLCoords(geometry [][]float64) string {
	parts := make([]string, len(geometry))
	for i, pt := range geometry {
		parts[i] = fmt.Sprintf("%f,%f,0", pt[0], pt[1])
	}
	return strings.Join(parts, "\n")
}

func marshalKML(doc Document) ([]byte, error) {
	outBytes, err := xml.MarshalIndent(KML{
		Xmlns: "http://www.opengis.net/kml/2.2",
		Doc:   doc,
	}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal kml: %w", err)
	}
	return append([]byte(xml.Header), outBytes...), nil
}
