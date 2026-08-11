package viewerchat

import (
	"strings"
	"testing"
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
