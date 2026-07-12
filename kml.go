package main

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/example/tripmap/routing"
)

type Stop struct {
	Name     string `yaml:"name"`
	Lat, Lon float64
}

type Day struct {
	Day   int    `yaml:"day"`
	Title string `yaml:"title"`
	Stops []Stop `yaml:"stops"`
}

type Trip struct {
	Trip string `yaml:"trip"`
	Days []Day  `yaml:"days"`
}

type KML struct {
	XMLName xml.Name `xml:"kml"`
	Xmlns   string   `xml:"xmlns,attr"`
	Doc     Document `xml:"Document"`
}

type Document struct {
	Name    string   `xml:"name"`
	Folders []Folder `xml:"Folder"`
}

type Folder struct {
	Name       string      `xml:"name"`
	Placemarks []Placemark `xml:"Placemark"`
}

type Placemark struct {
	Name  string `xml:"name"`
	Point *Point `xml:"Point,omitempty"`
	Line  *Line  `xml:"LineString,omitempty"`
}

type Point struct {
	Coordinates string `xml:"coordinates"`
}

type Line struct {
	Tess        int    `xml:"tessellate"`
	Coordinates string `xml:"coordinates"`
}

func buildDocument(ctx context.Context, t Trip, routeMode string) (Document, error) {
	doc := Document{Name: t.Trip}
	for _, d := range t.Days {
		f, err := buildFolder(ctx, d, routeMode)
		if err != nil {
			return Document{}, fmt.Errorf("day %d: %w", d.Day, err)
		}
		doc.Folders = append(doc.Folders, f)
	}
	return doc, nil
}

func buildFolder(ctx context.Context, d Day, routeMode string) (Folder, error) {
	f := Folder{Name: fmt.Sprintf("Day %d - %s", d.Day, d.Title)}
	for _, s := range d.Stops {
		f.Placemarks = append(f.Placemarks, Placemark{
			Name:  s.Name,
			Point: &Point{Coordinates: stopCoords(s)},
		})
	}
	if len(d.Stops) >= 2 {
		coords, err := routeCoords(ctx, d.Stops, routeMode)
		if err != nil {
			return Folder{}, err
		}
		f.Placemarks = append(f.Placemarks, Placemark{
			Name: "Route",
			Line: &Line{Tess: 1, Coordinates: coords},
		})
	}
	return f, nil
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
	var coords strings.Builder
	for i, s := range stops {
		if i > 0 {
			coords.WriteByte(' ')
		}
		coords.WriteString(stopCoords(s))
	}
	return coords.String()
}

func geometryToKMLCoords(geometry [][]float64) string {
	var coords strings.Builder
	for i, pt := range geometry {
		if i > 0 {
			coords.WriteByte(' ')
		}
		fmt.Fprintf(&coords, "%f,%f,0", pt[0], pt[1])
	}
	return coords.String()
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
