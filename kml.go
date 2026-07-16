package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/routebuild"
)

type Stop = itinerary.Stop
type Day = itinerary.Day
type Trip = itinerary.Trip
type RouteOptions = routebuild.RouteOptions

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


func stopKey(s Stop) string { return routebuild.StopKey(s) }
func mapPoints(d Day) []Stop { return routebuild.MapPoints(d) }
func viewerDayStops(d Day) []Stop { return routebuild.ViewerDayStops(d) }
func effectiveRoutePoints(d Day) []Stop { return routebuild.EffectiveRoutePoints(d) }

func buildRouteLines(ctx context.Context, d Day, pts []Stop, opts RouteOptions) ([]Placemark, error) {
	segs, err := routebuild.BuildRouteSegments(ctx, d, pts, opts)
	if err != nil {
		return nil, err
	}
	lines := make([]Placemark, len(segs))
	for i, s := range segs {
		lines[i] = routeLinePlacemark(s.Name, routebuild.GeometryToKMLCoords(s.Geometry, opts.CoordPrecision), "#"+s.Style)
	}
	return lines, nil
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
	f := Folder{Name: itinerary.DayFolderName(d), Description: d.Notes}

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
	return routebuild.RouteGeometryCoords(ctx, stops, mode, opts)
}

func formatCoords(lon, lat float64, precision int) string {
	return routebuild.FormatCoords(lon, lat, precision)
}

func straightLineCoords(stops []Stop, precision int) string {
	parts := make([]string, len(stops))
	for i, s := range stops {
		parts[i] = formatCoords(s.Lon, s.Lat, precision)
	}
	return strings.Join(parts, "\n")
}

func geometryToKMLCoords(geometry [][]float64, precision int) string {
	return routebuild.GeometryToKMLCoords(geometry, precision)
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
