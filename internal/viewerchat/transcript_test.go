package viewerchat

import (
	"context"
	"strings"
	"testing"

	"github.com/yaronf/tripmap/internal/itinerary"
)

func TestCurateMessagesShort(t *testing.T) {
	msgs := []ClientMessage{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
	}
	c := curateMessages(msgs)
	if c.Summary != "" || len(c.Messages) != 2 {
		t.Fatalf("%+v", c)
	}
}

func TestNeutralizePriorAddBeforeRemove(t *testing.T) {
	// Hayes Common: user asks to remove after assistant claimed an add — do not keep
	// the raw "I added…" prose in the model window.
	msgs := []ClientMessage{
		{Role: "user", Content: "Add Hayes Common for lunch on day 3"},
		{Role: "assistant", Content: "I've added Hayes Common as a lunch stop on Day 3."},
		{Role: "user", Content: "Remove the restaurant"},
	}
	c := curateMessages(msgs)
	if len(c.Messages) != 3 {
		t.Fatalf("%+v", c)
	}
	if !strings.HasPrefix(c.Messages[1].Content, "[Prior turn") {
		t.Fatalf("want neutralized prior add, got %q", c.Messages[1].Content)
	}
	if c.Messages[2].Content != "Remove the restaurant" {
		t.Fatalf("user ask must stay intact: %q", c.Messages[2].Content)
	}
	ws := buildWorkingState(c.Messages, "")
	if ws.ActiveIntent["kind"] != "remove" {
		t.Fatalf("want remove kind, got %+v", ws.ActiveIntent)
	}
}

func TestCurateMessagesSummarizesOlder(t *testing.T) {
	msgs := make([]ClientMessage, 0, 20)
	for i := 0; i < 10; i++ {
		msgs = append(msgs, ClientMessage{Role: "user", Content: "ask-" + string(rune('A'+i))})
		msgs = append(msgs, ClientMessage{Role: "assistant", Content: "Updated day " + string(rune('A'+i))})
	}
	c := curateMessages(msgs)
	if c.Summary == "" {
		t.Fatal("expected summary")
	}
	if !strings.Contains(c.Summary, "User asked") {
		t.Fatalf("summary=%q", c.Summary)
	}
	if len(c.Messages) != verbatimMessageCap {
		t.Fatalf("got %d recent", len(c.Messages))
	}
	if c.Messages[len(c.Messages)-1].Content != "Updated day J" {
		t.Fatalf("last=%q", c.Messages[len(c.Messages)-1].Content)
	}
}

func TestBuildTripFragmentNeighbors(t *testing.T) {
	body := []byte(`
trip: T
schema_version: 2
places:
  a: {title: A, type: overnight}
  b: {title: B, type: via}
  c: {title: C, type: overnight}
  d: {title: D, type: overnight}
days:
  - day: 1
    title: One
    route: [{place: a, type: overnight}, {place: b, type: via}, {place: c, type: overnight}]
  - day: 2
    title: Two
    route: [{place: c, type: overnight}, {place: d, type: overnight}]
  - day: 3
    title: Three
    route: [{place: d, type: overnight}]
`)
	frag, err := BuildTripFragment(body, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(frag.Days) != 3 {
		t.Fatalf("%+v", frag)
	}
	if frag.Days[0].Day != 1 || frag.Days[1].Day != 2 || frag.Days[2].Day != 3 {
		t.Fatalf("%+v", frag.Days)
	}
}

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
	// Center on day 1: window is 1..2 — includes far via day 2 stops; excludes only day 10 unique content.
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
	trip, err := itinerary.ParseYAML(scoped)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := trip.Places["far"]; !ok {
		t.Fatalf("expected stop place far: %#v", trip.Places)
	}
	if _, ok := trip.Places["a"]; !ok {
		t.Fatalf("expected place a: %#v", trip.Places)
	}
	// Day 10 not in window — but place a is still included from day 1.
	if len(trip.Days) != 2 {
		t.Fatalf("days=%d", len(trip.Days))
	}
}

func TestHandleGetTripYAMLScope(t *testing.T) {
	body := []byte(`
trip: T
schema_version: 2
places:
  a: {title: A, type: overnight}
  b: {title: B, type: overnight}
  z: {title: Z, type: attraction}
days:
  - day: 1
    title: One
    route: [{place: a, type: overnight}, {place: b, type: overnight}]
  - day: 2
    title: Two
    route: [{place: b, type: overnight}]
`)
	ops := &memOps{yaml: body}
	a := &Agent{ops: ops}

	// Default with viewer day → day scope.
	res, err := handleGetTripYAML(context.Background(), a, TurnInput{TripID: "t1", Day: 1}, `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Content, "# scope: day") {
		t.Fatalf("want day scope: %s", res.Content)
	}
	if strings.Contains(res.Content, "title: Z") {
		t.Fatalf("should omit unused place z: %s", res.Content)
	}

	// Explicit full.
	res, err = handleGetTripYAML(context.Background(), a, TurnInput{TripID: "t1", Day: 1}, `{"scope":"full"}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, "# scope: day") {
		t.Fatalf("want full: %s", res.Content)
	}
	if !strings.Contains(res.Content, "title: Z") {
		t.Fatalf("full should include z: %s", res.Content)
	}

	// No viewer day and no args → full.
	res, err = handleGetTripYAML(context.Background(), a, TurnInput{TripID: "t1"}, `{}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.Content, "# scope: day") {
		t.Fatalf("want full without day: %s", res.Content)
	}
}

func TestContinuityWarnings(t *testing.T) {
	body := []byte(`
trip: T
schema_version: 2
places:
  a: {title: A, type: overnight}
  b: {title: B, type: overnight}
  c: {title: C, type: overnight}
days:
  - day: 1
    title: One
    route: [{place: a, type: overnight}, {place: b, type: overnight}]
  - day: 2
    title: Two
    route: [{place: c, type: overnight}, {place: a, type: overnight}]
`)
	warns := ContinuityWarnings(body, []int{1})
	if len(warns) != 1 || !strings.Contains(warns[0], "b") {
		t.Fatalf("%v", warns)
	}
}
