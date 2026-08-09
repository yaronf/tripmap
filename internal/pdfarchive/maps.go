package pdfarchive

import (
	"bytes"
	"fmt"
	"image/color"
	"image/png"

	sm "github.com/flopp/go-staticmaps"
	"github.com/golang/geo/s2"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/routebuild"
)

// MapRenderer renders a static map image. Tests can substitute a no-tile renderer.
type MapRenderer interface {
	Render(paths []MapPath, markers []MapMarker, width, height int) ([]byte, error)
}

// MapPath is a polyline in lat/lon order for static maps.
type MapPath struct {
	LatLngs [][2]float64 // lat, lon
	Color   color.RGBA
	Weight  float64
}

// MapMarker is a point marker.
type MapMarker struct {
	Lat, Lon float64
	Color    color.RGBA
	Size     float64
}

// StaticMapRenderer uses go-staticmaps (OSM/Carto tiles by default).
type StaticMapRenderer struct {
	// If true, skip downloading tiles (geometry only) — for tests / offline.
	NoTiles bool
}

func (r StaticMapRenderer) Render(paths []MapPath, markers []MapMarker, width, height int) ([]byte, error) {
	if width <= 0 {
		width = 800
	}
	if height <= 0 {
		height = 500
	}
	ctx := sm.NewContext()
	ctx.SetSize(width, height)
	if r.NoTiles {
		ctx.SetTileProvider(sm.NewTileProviderNone())
	} else {
		ctx.SetTileProvider(sm.NewTileProviderCartoLight())
	}
	for _, p := range paths {
		if len(p.LatLngs) < 2 {
			continue
		}
		pos := make([]s2.LatLng, len(p.LatLngs))
		for i, ll := range p.LatLngs {
			pos[i] = s2.LatLngFromDegrees(ll[0], ll[1])
		}
		w := p.Weight
		if w <= 0 {
			w = 3
		}
		col := p.Color
		if col.A == 0 {
			col = color.RGBA{R: 30, G: 100, B: 200, A: 255}
		}
		ctx.AddObject(sm.NewPath(pos, col, w))
	}
	for _, m := range markers {
		col := m.Color
		if col.A == 0 {
			col = color.RGBA{R: 200, G: 40, B: 40, A: 255}
		}
		sz := m.Size
		if sz <= 0 {
			sz = 12
		}
		ctx.AddObject(sm.NewMarker(s2.LatLngFromDegrees(m.Lat, m.Lon), col, sz))
	}
	img, err := ctx.Render()
	if err != nil {
		return nil, fmt.Errorf("render map: %w", err)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

var dayPalette = []color.RGBA{
	{R: 30, G: 100, B: 200, A: 255},
	{R: 200, G: 80, B: 40, A: 255},
	{R: 40, G: 140, B: 80, A: 255},
	{R: 140, G: 60, B: 160, A: 255},
	{R: 200, G: 150, B: 30, A: 255},
	{R: 40, G: 140, B: 160, A: 255},
	{R: 180, G: 60, B: 90, A: 255},
	{R: 80, G: 80, B: 180, A: 255},
}

func styleColor(style string, dayIdx int) color.RGBA {
	switch style {
	case "hikeLine":
		return color.RGBA{R: 40, G: 140, B: 70, A: 255}
	case "ferryLine":
		return color.RGBA{R: 220, G: 120, B: 40, A: 255}
	default:
		return dayPalette[dayIdx%len(dayPalette)]
	}
}

func geomToLatLngs(geom [][]float64) [][2]float64 {
	out := make([][2]float64, 0, len(geom))
	for _, c := range geom {
		if len(c) < 2 {
			continue
		}
		out = append(out, [2]float64{c[1], c[0]}) // lat, lon
	}
	return out
}

func overnightMarkers(d itinerary.Day, col color.RGBA) []MapMarker {
	var markers []MapMarker
	seen := map[string]bool{}
	add := func(s itinerary.Stop) {
		if s.Type != "overnight" && s.Type != "depart" {
			return
		}
		key := routebuild.StopKey(s)
		if seen[key] {
			return
		}
		seen[key] = true
		markers = append(markers, MapMarker{Lat: s.Lat, Lon: s.Lon, Color: col, Size: 10})
	}
	for _, s := range d.Route {
		add(s)
	}
	for _, s := range d.Stops {
		add(s)
	}
	return markers
}

func dayStopMarkers(stops []itinerary.Stop) []MapMarker {
	var markers []MapMarker
	seen := map[string]bool{}
	for _, s := range stops {
		if s.Type == "via" {
			continue
		}
		key := routebuild.StopKey(s)
		if seen[key] {
			continue
		}
		seen[key] = true
		col := color.RGBA{R: 200, G: 40, B: 40, A: 255}
		switch s.Type {
		case "overnight":
			col = color.RGBA{R: 30, G: 80, B: 180, A: 255}
		case "depart":
			col = color.RGBA{R: 100, G: 100, B: 100, A: 255}
		case "attraction", "hike":
			col = color.RGBA{R: 40, G: 140, B: 70, A: 255}
		}
		markers = append(markers, MapMarker{Lat: s.Lat, Lon: s.Lon, Color: col, Size: 11})
	}
	return markers
}
