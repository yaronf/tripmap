// Package pdfarchive builds a printable PDF archive of a tripmap itinerary.
package pdfarchive

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"
	"github.com/yaronf/tripmap/internal/bundle"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/routebuild"
	"github.com/yaronf/tripmap/internal/store"
)

// Options controls PDF archive generation.
type Options struct {
	Route    routebuild.RouteOptions
	Maps     MapRenderer
	PhotoDir string // resolve relative photo paths; may be empty
}

type dayBuilt struct {
	day      itinerary.Day
	segs     []routebuild.Segment
	stops    []itinerary.Stop
	driveM   float64
	driveS   float64
	mapPNG   []byte
	comment  string
	dayPhoto []byte
}

// Build writes a PDF archive to outPath.
func Build(ctx context.Context, trip itinerary.Trip, notes store.NotesDoc, outPath string, opts Options) error {
	if opts.Maps == nil {
		opts.Maps = StaticMapRenderer{}
	}
	if opts.Route.Units == "" {
		opts.Route.Units = "km"
	}
	if opts.Route.Mode == "" {
		opts.Route.Mode = "straight"
	}

	days := make([]dayBuilt, 0, len(trip.Days))
	var overviewPaths []MapPath
	var overviewMarkers []MapMarker

	for i, d := range trip.Days {
		db := dayBuilt{day: d, stops: routebuild.ViewerDayStops(d)}
		if notes.Days != nil {
			db.comment = strings.TrimSpace(notes.Days[strconv.Itoa(d.Day)])
		}
		rp := routebuild.EffectiveRoutePoints(d)
		if len(rp) >= 2 {
			segs, err := routebuild.BuildRouteSegments(ctx, d, rp, opts.Route)
			if err != nil {
				return fmt.Errorf("day %d route: %w", d.Day, err)
			}
			db.segs = segs
			col := dayPalette[i%len(dayPalette)]
			for _, seg := range segs {
				ll := geomToLatLngs(seg.Geometry)
				if len(ll) < 2 {
					continue
				}
				overviewPaths = append(overviewPaths, MapPath{LatLngs: ll, Color: styleColor(seg.Style, i), Weight: 2.5})
				if seg.Style == "driveLine" {
					db.driveM += seg.DistanceMeters
					db.driveS += seg.DurationSeconds
				}
			}
			overviewMarkers = append(overviewMarkers, overnightMarkers(d, col)...)
		}
		// Per-day map
		var dayPaths []MapPath
		for _, seg := range db.segs {
			ll := geomToLatLngs(seg.Geometry)
			if len(ll) >= 2 {
				dayPaths = append(dayPaths, MapPath{LatLngs: ll, Color: styleColor(seg.Style, i), Weight: 3.5})
			}
		}
		markers := dayStopMarkers(db.stops)
		if len(dayPaths) > 0 || len(markers) > 0 {
			png, err := opts.Maps.Render(dayPaths, markers, 800, 480)
			if err != nil {
				return fmt.Errorf("day %d map: %w", d.Day, err)
			}
			db.mapPNG = png
		}
		if p := strings.TrimSpace(d.Photo); p != "" {
			if b, err := loadPhoto(p, opts.PhotoDir); err == nil {
				db.dayPhoto = b
			}
		}
		days = append(days, db)
	}

	var overviewPNG []byte
	if len(overviewPaths) > 0 || len(overviewMarkers) > 0 {
		var err error
		overviewPNG, err = opts.Maps.Render(overviewPaths, overviewMarkers, 900, 560)
		if err != nil {
			return fmt.Errorf("overview map: %w", err)
		}
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil && filepath.Dir(outPath) != "." {
		return err
	}
	return writePDF(trip, days, overviewPNG, outPath, opts.Route.Units)
}

func loadPhoto(src, photoDir string) ([]byte, error) {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return nil, fmt.Errorf("skip remote photo")
	}
	path := src
	if !filepath.IsAbs(src) && photoDir != "" {
		path = filepath.Join(photoDir, src)
	}
	return os.ReadFile(path)
}

func writePDF(trip itinerary.Trip, days []dayBuilt, overviewPNG []byte, outPath, units string) error {
	pdf := fpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false) // keep text streams searchable / testable
	pdf.SetTitle(trip.Trip, true)
	pdf.SetAuthor("tripmap", true)
	registerFonts(pdf)
	genStamp := time.Now().UTC().Format("2006-01-02 15:04 UTC")
	// Footer must use SetFooterFunc (inFooter=true). A manual Cell near the
	// bottom sits past pageBreakTrigger and would auto-AddPage each time.
	pdf.SetFooterFunc(func() {
		pdf.SetY(-12)
		pdf.SetFont(fontFamily, "", 8)
		pdf.SetTextColor(100, 100, 100)
		pdf.CellFormat(0, 5, fmt.Sprintf("Generated %s · © OpenStreetMap contributors, CARTO", genStamp),
			"", 0, "C", false, 0, "")
	})

	// Cover
	pdf.AddPage()
	pdf.SetFont(fontFamily, "B", 22)
	pdf.MultiCell(0, 10, trip.Trip, "", "L", false)
	pdf.Ln(2)
	pdf.SetFont(fontFamily, "", 11)
	if trip.Description != "" {
		pdf.MultiCell(0, 6, trip.Description, "", "L", false)
		pdf.Ln(2)
	}
	meta := fmt.Sprintf("%d days", len(trip.Days))
	if trip.Start != "" {
		meta = "Start " + trip.Start + " · " + meta
	}
	pdf.SetTextColor(80, 80, 80)
	pdf.Cell(0, 6, meta)
	pdf.Ln(7)
	pdf.SetFont(fontFamily, "", 10)
	pdf.SetTextColor(80, 80, 80)
	pdf.Write(6, "Itinerary generated using ")
	pdf.SetTextColor(30, 90, 180)
	pdf.WriteLinkString(6, "Tripmap", "https://tripmap.sheffer.org")
	pdf.SetTextColor(0, 0, 0)
	placeGitHubIcon(pdf, 5)
	pdf.Ln(10)
	if len(overviewPNG) > 0 {
		pdf.SetFont(fontFamily, "B", 12)
		pdf.Cell(0, 7, "Overview")
		pdf.Ln(8)
		if err := embedPNG(pdf, overviewPNG, 190, 118); err != nil {
			return err
		}
	}

	for _, db := range days {
		pdf.AddPage()
		d := db.day
		micro := fmt.Sprintf("Day %d", d.Day)
		if d.Date != "" {
			micro += " · " + d.Date
		}
		pdf.SetFont(fontFamily, "", 10)
		pdf.SetTextColor(100, 100, 100)
		pdf.Cell(0, 5, micro)
		pdf.Ln(6)
		pdf.SetTextColor(0, 0, 0)
		pdf.SetFont(fontFamily, "B", 16)
		pdf.MultiCell(0, 8, d.Title, "", "L", false)
		pdf.Ln(1)

		var flags []string
		if d.Hike {
			flags = append(flags, "hike")
		}
		if d.Ferry {
			flags = append(flags, "ferry")
		}
		if db.driveM > 0 {
			flags = append(flags, formatDrive(db.driveM, db.driveS, units))
		}
		if len(flags) > 0 {
			pdf.SetFont(fontFamily, "", 10)
			pdf.SetTextColor(60, 60, 60)
			pdf.Cell(0, 5, strings.Join(flags, " · "))
			pdf.Ln(7)
			pdf.SetTextColor(0, 0, 0)
		}

		if strings.TrimSpace(d.Notes) != "" {
			sectionHeading(pdf, "Notes")
			pdf.SetFont(fontFamily, "", 10)
			pdf.MultiCell(0, 5, d.Notes, "", "L", false)
			pdf.Ln(3)
		}
		if db.comment != "" {
			sectionHeading(pdf, "Comments")
			pdf.SetFont(fontFamily, "", 10)
			pdf.MultiCell(0, 5, db.comment, "", "L", false)
			pdf.Ln(3)
		}
		if len(db.stops) > 0 {
			sectionHeading(pdf, "Stops")
			pdf.SetFont(fontFamily, "", 10)
			for _, s := range db.stops {
				line := s.Name
				if s.Type != "" {
					line += " (" + s.Type + ")"
				}
				pdf.MultiCell(0, 5, "• "+line, "", "L", false)
				if strings.TrimSpace(s.Notes) != "" {
					pdf.SetX(pdf.GetX() + 4)
					pdf.SetTextColor(70, 70, 70)
					pdf.MultiCell(0, 4, s.Notes, "", "L", false)
					pdf.SetTextColor(0, 0, 0)
				}
			}
			pdf.Ln(2)
		}
		if len(db.mapPNG) > 0 {
			sectionHeading(pdf, "Map")
			if err := embedPNG(pdf, db.mapPNG, 190, 114); err != nil {
				return err
			}
		}
		if len(db.dayPhoto) > 0 {
			sectionHeading(pdf, "Photo")
			_ = embedImageBytes(pdf, db.dayPhoto, 120, 80)
		}
	}

	return pdf.OutputFileAndClose(outPath)
}

func sectionHeading(pdf *fpdf.Fpdf, title string) {
	pdf.SetFont(fontFamily, "B", 11)
	pdf.Cell(0, 6, title)
	pdf.Ln(6)
}

// placeGitHubIcon draws the GitHub mark after the current text position and
// links it to the tripmap repo.
func placeGitHubIcon(pdf *fpdf.Fpdf, sizeMM float64) {
	const name = "github-mark"
	opt := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
	if pdf.GetImageInfo(name) == nil {
		pdf.RegisterImageOptionsReader(name, opt, bytes.NewReader(githubMarkPNG))
	}
	x := pdf.GetX() + 2
	y := pdf.GetY() + 0.5
	pdf.ImageOptions(name, x, y, sizeMM, sizeMM, false, opt, 0, githubRepoURL)
	pdf.SetX(x + sizeMM)
}

func formatDrive(meters, seconds float64, units string) string {
	dist := bundle.DistanceInUnits(meters, units)
	unit := "km"
	if units == "mi" {
		unit = "mi"
	}
	s := fmt.Sprintf("%.1f %s", dist, unit)
	if seconds > 0 {
		min := int(math.Round(seconds / 60))
		s += fmt.Sprintf(" · %d min", min)
	}
	return s
}

func embedPNG(pdf *fpdf.Fpdf, pngBytes []byte, maxW, maxH float64) error {
	name := fmt.Sprintf("img%d", pdf.PageNo()*1000+int(pdf.GetY()*10))
	opt := fpdf.ImageOptions{ImageType: "PNG", ReadDpi: true}
	pdf.RegisterImageOptionsReader(name, opt, bytes.NewReader(pngBytes))
	info := pdf.GetImageInfo(name)
	if info == nil {
		return fmt.Errorf("register png")
	}
	w := maxW
	h := info.Height() * w / info.Width()
	if h > maxH {
		h = maxH
		w = info.Width() * h / info.Height()
	}
	// flow=true advances Y and page-breaks before placing if needed
	pdf.ImageOptions(name, pdf.GetX(), pdf.GetY(), w, h, true, opt, 0, "")
	pdf.Ln(2)
	return nil
}

func embedImageBytes(pdf *fpdf.Fpdf, data []byte, maxW, maxH float64) error {
	// Detect type by magic
	kind := ""
	switch {
	case bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4e, 0x47}):
		kind = "PNG"
	case bytes.HasPrefix(data, []byte{0xff, 0xd8}):
		kind = "JPG"
	default:
		return fmt.Errorf("unsupported image")
	}
	name := fmt.Sprintf("photo%d", pdf.PageNo()*1000+int(pdf.GetY()*10))
	opt := fpdf.ImageOptions{ImageType: kind, ReadDpi: true}
	pdf.RegisterImageOptionsReader(name, opt, bytes.NewReader(data))
	info := pdf.GetImageInfo(name)
	if info == nil {
		return fmt.Errorf("register image")
	}
	w := maxW
	h := info.Height() * w / info.Width()
	if h > maxH {
		h = maxH
		w = info.Width() * h / info.Height()
	}
	pdf.ImageOptions(name, pdf.GetX(), pdf.GetY(), w, h, true, opt, 0, "")
	pdf.Ln(2)
	return nil
}
