package pdfarchive_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf16"

	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/pdfarchive"
	"github.com/yaronf/tripmap/internal/routebuild"
	"github.com/yaronf/tripmap/internal/store"
)

func TestBuildPDFStraightRouteIncludesComments(t *testing.T) {
	yaml := []byte(`
schema_version: 2
trip: Test Archive
description: PDF fixture
start: "2026-06-01"
places:
  a: { title: Alpha, lat: 52.1, lon: 5.1, type: overnight }
  b: { title: Beta, lat: 52.2, lon: 5.2, type: overnight }
  c: { title: Lookout, lat: 52.15, lon: 5.15, type: attraction }
days:
  - day: 1
    title: "Wānaka → Te Anau"
    notes: Pack snacks.
    route:
      - { place: a }
      - { place: b }
    stops:
      - { place: c }
  - day: 2
    title: Second day
    route:
      - { place: b }
      - { place: a }
`)
	trip, err := itinerary.ParseYAML(yaml)
	if err != nil {
		t.Fatal(err)
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		t.Fatal(err)
	}
	if err := itinerary.ResolvePlaces(&trip); err != nil {
		t.Fatal(err)
	}
	if err := itinerary.ResolveDayDates(&trip); err != nil {
		t.Fatal(err)
	}

	notes := store.NotesDoc{Days: map[string]string{
		"1": "Loved the lookout - archive me.",
	}}

	dir := t.TempDir()
	out := filepath.Join(dir, "trip.pdf")
	err = pdfarchive.Build(context.Background(), trip, notes, out, pdfarchive.Options{
		Route: routebuild.RouteOptions{Mode: "straight", Units: "km", CoordPrecision: 5},
		Maps:  pdfarchive.StaticMapRenderer{NoTiles: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 500 {
		t.Fatalf("pdf too small: %d bytes", len(b))
	}
	if !bytes.HasPrefix(b, []byte("%PDF")) {
		t.Fatalf("not a PDF: %q", b[:min(20, len(b))])
	}
	// UTF-8 fonts emit UTF-16BE in content streams.
	for _, s := range []string{
		"Loved the lookout - archive me.",
		"Pack snacks",
		"Test Archive",
		"Wānaka → Te Anau",
	} {
		if !bytes.Contains(b, utf16BE(s)) {
			t.Fatalf("expected %q (UTF-16) in PDF (%d bytes)", s, len(b))
		}
	}
	if bytes.Contains(b, utf16BE("W.naka")) || bytes.Contains(b, []byte("W.naka")) {
		t.Fatalf("macron must not collapse to a period")
	}
}

func utf16BE(s string) []byte {
	u := utf16.Encode([]rune(s))
	out := make([]byte, 0, len(u)*2)
	for _, c := range u {
		out = append(out, byte(c>>8), byte(c))
	}
	return out
}
