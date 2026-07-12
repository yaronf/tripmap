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
//	hut             backcountry hut: map placemark + hike route point
//	via             route-shaping only: on the line, no placemark
//	attraction      map placemark only, not on the route
//	viewpoint       map placemark only, not on the route
//	trailhead       trail car park: map placemark + route point
//	ferry_terminal  map placemark + route point (draw with ferry styling)
//	airport         airport: map placemark + route point
type Stop struct {
	Name     string `yaml:"name"`
	Lat, Lon float64
	Type     string `yaml:"type,omitempty"`
	Notes    string `yaml:"notes,omitempty"`
}

// Day is one day of the trip. Route explicitly defines the line, while Stops
// defines additional map placemarks. Hike days may combine OSRM driving
// approaches with straight-line trail segments.
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

// RouteOptions controls road routing and KML coordinate detail.
type RouteOptions struct {
	Mode           string
	SimplifyMeters float64
	CoordPrecision int
	Flatten        bool // flat placemarks for Google My Maps (no Folders)
}

type KML struct {
	XMLName xml.Name `xml:"kml"`
	Xmlns   string   `xml:"xmlns,attr"`
	Doc     Document `xml:"Document"`
}

type Document struct {
	Name        string      `xml:"name"`
	Description string      `xml:"description,omitempty"`
	Styles      []Style     `xml:"Style,omitempty"`
	Folders     []Folder    `xml:"Folder,omitempty"`
	Placemarks  []Placemark `xml:"Placemark,omitempty"`
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
	{"hut", "ff008800", "http://maps.google.com/mapfiles/kml/shapes/campfire.png"},
	{"attraction", "ff00aaff", "http://maps.google.com/mapfiles/kml/shapes/star.png"},
	{"viewpoint", "ffff8800", "http://maps.google.com/mapfiles/kml/shapes/camera.png"},
	{"trailhead", "ff00aa00", "http://maps.google.com/mapfiles/kml/shapes/hiker.png"},
	{"ferry_terminal", "ffaa5500", "http://maps.google.com/mapfiles/kml/shapes/ferry.png"},
	{"airport", "ff3333cc", "http://maps.google.com/mapfiles/kml/shapes/airports.png"},
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

func stopKey(s Stop) string {
	return fmt.Sprintf("%.6f|%.6f", s.Lat, s.Lon)
}

func sameStop(a, b Stop) bool {
	return stopKey(a) == stopKey(b)
}

// mapPoints returns placemark candidates for a day from Stops and Route,
// excluding via points and de-duplicating by location within the day.
func mapPoints(d Day) []Stop {
	var pts []Stop
	seen := map[string]bool{}
	add := func(s Stop) {
		if s.Type == "via" {
			return
		}
		key := stopKey(s)
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

// effectiveRoutePoints builds the ordered list of points used to draw lines.
// On hike days, prepends lodging from stops when the route does not already
// start there, and falls back to stops when no route is defined.
func effectiveRoutePoints(d Day) []Stop {
	pts := append([]Stop{}, d.Route...)
	if len(pts) == 0 && d.Hike {
		for _, s := range d.Stops {
			if s.Type != "attraction" {
				pts = append(pts, s)
			}
		}
	}
	if !d.Hike || len(pts) == 0 {
		return pts
	}
	for _, s := range d.Stops {
		if s.Type == "overnight" && !sameStop(s, pts[0]) {
			return append([]Stop{s}, pts...)
		}
	}
	return pts
}

func isTrailPoint(t string) bool {
	switch t {
	case "trailhead", "hut", "viewpoint", "attraction":
		return true
	default:
		return false
	}
}

func isDrivingPoint(t string) bool {
	switch t {
	case "overnight", "ferry_terminal", "airport", "via", "":
		return true
	default:
		return false
	}
}

// isDrivingSegment reports whether a pair should use road routing on hike days.
func isDrivingSegment(a, b Stop) bool {
	return (isDrivingPoint(a.Type) && isTrailPoint(b.Type)) ||
		(isTrailPoint(a.Type) && isDrivingPoint(b.Type))
}

func isFerrySegment(a, b Stop) bool {
	return a.Type == "ferry_terminal" && b.Type == "ferry_terminal"
}

func buildDocument(ctx context.Context, t Trip, opts RouteOptions) (Document, error) {
	doc := Document{Name: t.Trip, Description: t.Description}
	seen := map[string]bool{}
	for _, d := range t.Days {
		f, err := buildFolder(ctx, d, opts, seen)
		if err != nil {
			return Document{}, fmt.Errorf("day %d: %w", d.Day, err)
		}
		doc.Folders = append(doc.Folders, f)
	}
	if opts.Flatten {
		doc = flattenForMyMaps(doc)
	}
	doc.Styles = usedStyles(doc.Folders, doc.Placemarks)
	return doc, nil
}

func buildFolder(ctx context.Context, d Day, opts RouteOptions, seen map[string]bool) (Folder, error) {
	f := Folder{Name: fmt.Sprintf("Day %d - %s", d.Day, d.Title), Description: d.Notes}

	for _, s := range mapPoints(d) {
		key := stopKey(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		pm := Placemark{
			Name:        s.Name,
			Description: s.Notes,
			Point:       &Point{Coordinates: formatCoords(s.Lon, s.Lat, opts.CoordPrecision)},
		}
		if id := styleForType(s.Type); id != "" {
			pm.StyleURL = "#" + id
		}
		f.Placemarks = append(f.Placemarks, pm)
	}

	rp := effectiveRoutePoints(d)
	if len(rp) < 2 {
		return f, nil
	}

	lines, err := buildRouteLines(ctx, d, rp, opts)
	if err != nil {
		return Folder{}, err
	}
	f.Placemarks = append(f.Placemarks, lines...)
	return f, nil
}

func buildRouteLines(ctx context.Context, d Day, pts []Stop, opts RouteOptions) ([]Placemark, error) {
	if d.Ferry {
		return buildSegmentLines(ctx, pts, opts, func(a, b Stop) (mode, style, name string) {
			if isFerrySegment(a, b) {
				return "straight", "#ferryLine", "Ferry"
			}
			return opts.Mode, "#driveLine", "Drive"
		})
	}

	if !d.Hike {
		coords, err := routeCoords(ctx, pts, opts.Mode, opts)
		if err != nil {
			return nil, err
		}
		return []Placemark{routeLinePlacemark("Route", coords, "#driveLine")}, nil
	}

	return buildSegmentLines(ctx, pts, opts, func(a, b Stop) (mode, style, name string) {
		if isDrivingSegment(a, b) {
			return opts.Mode, "#driveLine", "Drive"
		}
		return "straight", "#hikeLine", "Hike"
	})
}

func buildSegmentLines(ctx context.Context, pts []Stop, opts RouteOptions, classify func(a, b Stop) (mode, style, name string)) ([]Placemark, error) {
	var lines []Placemark
	for i := 0; i < len(pts)-1; i++ {
		seg := []Stop{pts[i], pts[i+1]}
		mode, style, name := classify(seg[0], seg[1])
		coords, err := routeCoords(ctx, seg, mode, opts)
		if err != nil {
			return nil, err
		}
		lines = append(lines, routeLinePlacemark(name, coords, style))
	}
	if len(lines) == 1 {
		lines[0].Name = "Route"
	}
	return lines, nil
}

func routeLinePlacemark(name, coords, style string) Placemark {
	return Placemark{
		Name:     name,
		StyleURL: style,
		Line: &Line{
			Tess:         1,
			AltitudeMode: "clampToGround",
			Coordinates:  coords,
		},
	}
}

// usedStyles returns Style definitions referenced by placemarks, in output order.
func usedStyles(folders []Folder, placemarks []Placemark) []Style {
	used := map[string]bool{}
	collect := func(pms []Placemark) {
		for _, pm := range pms {
			if pm.StyleURL != "" {
				used[strings.TrimPrefix(pm.StyleURL, "#")] = true
			}
		}
	}
	for _, f := range folders {
		collect(f.Placemarks)
	}
	collect(placemarks)

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

func routeCoords(ctx context.Context, stops []Stop, mode string, opts RouteOptions) (string, error) {
	switch mode {
	case "straight":
		return straightLineCoords(stops, opts.CoordPrecision), nil
	case "osrm":
		pts := make([]routing.Point, len(stops))
		for i, s := range stops {
			pts[i] = routing.Point{Lat: s.Lat, Lon: s.Lon}
		}
		overview := "full"
		if opts.SimplifyMeters > 0 {
			overview = "simplified"
		}
		route, err := routing.RouteOSRMOverview(ctx, pts, overview)
		if err != nil {
			return "", err
		}
		geom := routing.SimplifyGeometry(route.Geometry, opts.SimplifyMeters)
		return geometryToKMLCoords(geom, opts.CoordPrecision), nil
	default:
		return "", fmt.Errorf("unknown route mode %q (use straight or osrm)", mode)
	}
}

func formatCoords(lon, lat float64, precision int) string {
	if precision <= 0 {
		precision = 6
	}
	format := fmt.Sprintf("%%.%df,%%.%df,0", precision, precision)
	return fmt.Sprintf(format, lon, lat)
}

func straightLineCoords(stops []Stop, precision int) string {
	parts := make([]string, len(stops))
	for i, s := range stops {
		parts[i] = formatCoords(s.Lon, s.Lat, precision)
	}
	return strings.Join(parts, "\n")
}

func geometryToKMLCoords(geometry [][]float64, precision int) string {
	parts := make([]string, len(geometry))
	for i, pt := range geometry {
		parts[i] = formatCoords(pt[0], pt[1], precision)
	}
	return strings.Join(parts, "\n")
}

const myMapsMaxLinePoints = 499

func flattenForMyMaps(doc Document) Document {
	var flat []Placemark
	for _, f := range doc.Folders {
		if len(f.Placemarks) == 0 {
			continue
		}
		prefix := f.Name + ": "
		for _, pm := range f.Placemarks {
			pm.Name = prefix + pm.Name
			if pm.Description == "" && f.Description != "" {
				pm.Description = f.Description
			}
			flat = append(flat, splitLongLinePlacemark(pm, myMapsMaxLinePoints)...)
		}
	}
	doc.Folders = nil
	doc.Placemarks = flat
	return doc
}

func splitLongLinePlacemark(pm Placemark, maxPts int) []Placemark {
	if pm.Line == nil || maxPts <= 0 {
		return []Placemark{pm}
	}
	pts := parseCoordLines(pm.Line.Coordinates)
	if len(pts) <= maxPts {
		return []Placemark{pm}
	}

	var out []Placemark
	for start := 0; start < len(pts)-1; {
		end := start + maxPts
		if end >= len(pts) {
			end = len(pts) - 1
		}
		chunk := pts[start : end+1]
		part := pm
		if len(out) > 0 {
			part.Name = fmt.Sprintf("%s (%d)", pm.Name, len(out)+1)
		}
		part.Line = &Line{
			Tess:         pm.Line.Tess,
			AltitudeMode: pm.Line.AltitudeMode,
			Coordinates:  strings.Join(chunk, "\n"),
		}
		out = append(out, part)
		if end >= len(pts)-1 {
			break
		}
		start = end
	}
	return out
}

func parseCoordLines(coords string) []string {
	coords = strings.ReplaceAll(coords, "\r\n", "\n")
	return strings.FieldsFunc(coords, func(r rune) bool {
		return r == '\n'
	})
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
