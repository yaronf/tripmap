package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildKMLStraightGolden(t *testing.T) {
	input := filepath.Join("testdata", "itinerary.yaml")
	golden := filepath.Join("testdata", "trip_straight.golden.kml")

	b, err := os.ReadFile(input)
	if err != nil {
		t.Fatalf("read input: %v", err)
	}

	var trip Trip
	if err := yaml.Unmarshal(b, &trip); err != nil {
		t.Fatalf("parse yaml: %v", err)
	}

	doc, err := buildDocument(context.Background(), trip, "straight")
	if err != nil {
		t.Fatalf("build document: %v", err)
	}

	got, err := marshalKML(doc)
	if err != nil {
		t.Fatalf("marshal kml: %v", err)
	}

	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}

	if string(got) != string(want) {
		t.Fatalf("kml output differs from golden file %s", golden)
	}
}

func TestRunInvalidRouteMode(t *testing.T) {
	err := run([]string{
		"-input", filepath.Join("testdata", "itinerary.yaml"),
		"-output", t.TempDir() + "/out.kml",
		"-route", "invalid",
	})
	if err == nil {
		t.Fatal("expected error for invalid route mode")
	}
}
