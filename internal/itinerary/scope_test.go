package itinerary

import (
	"strings"
	"testing"
)

func TestBuildDayScopedYAML(t *testing.T) {
	body := []byte(`
trip: T
schema_version: 2
description: demo
places:
  a: {title: A, type: overnight}
  b: {title: B, type: via}
  c: {title: C, type: overnight}
  d: {title: D, type: overnight}
  far: {title: Far, type: attraction}
days:
  - day: 1
    title: One
    route: [{place: a, type: overnight}, {place: b, type: via}, {place: c, type: overnight}]
  - day: 2
    title: Two
    route: [{place: c, type: overnight}, {place: d, type: overnight}]
    stops: [{place: far, type: attraction}]
  - day: 3
    title: Three
    route: [{place: d, type: overnight}]
  - day: 10
    title: Distant
    route: [{place: a, type: overnight}]
`)
	scoped, err := BuildDayScopedYAML(body, 1)
	if err != nil {
		t.Fatal(err)
	}
	text := string(scoped)
	if !strings.Contains(text, "# scope: day") {
		t.Fatalf("missing header: %s", text)
	}
	if !strings.Contains(text, "title: One") || !strings.Contains(text, "title: Two") {
		t.Fatalf("expected days 1-2: %s", text)
	}
	if strings.Contains(text, "title: Three") || strings.Contains(text, "title: Distant") {
		t.Fatalf("unexpected days: %s", text)
	}
	trip, err := ParseYAML(scoped)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := trip.Places["far"]; !ok {
		t.Fatalf("expected stop place far: %#v", trip.Places)
	}
	if _, ok := trip.Places["a"]; !ok {
		t.Fatalf("expected place a: %#v", trip.Places)
	}
	if len(trip.Days) != 2 {
		t.Fatalf("days=%d", len(trip.Days))
	}
}
