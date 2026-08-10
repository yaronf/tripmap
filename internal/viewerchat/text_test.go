package viewerchat

import (
	"strings"
	"testing"
)

func TestCleanAssistantText_StripsOpenAICitationsAndDupes(t *testing.T) {
	chunk := "[Explore Queenstown's Surrounding Regions | Official Website](https://www.queenstownnz.co.nz/plan/surrounding-region/?utm_source=openai)I've successfully added a photo of Kingston to Day 22. If you need any further adjustments or assistance, feel free to let me know!"
	in := strings.Repeat(chunk, 8)
	got := cleanAssistantText(in)
	if strings.Contains(got, "utm_source=openai") {
		t.Fatalf("citation left behind: %q", got)
	}
	if strings.Count(got, "I've successfully added") != 1 {
		t.Fatalf("expected one success sentence, got %q", got)
	}
	if !strings.Contains(got, "Kingston") {
		t.Fatalf("lost content: %q", got)
	}
	if strings.Contains(strings.ToLower(got), "let me know") {
		t.Fatalf("filler left behind: %q", got)
	}
}

func TestCleanAssistantText_StripsStopsFiller(t *testing.T) {
	in := "Updated Day 22. If you need more information or want to adjust any stops, let me know!"
	got := cleanAssistantText(in)
	if got != "Updated Day 22." {
		t.Fatalf("got %q", got)
	}
}
