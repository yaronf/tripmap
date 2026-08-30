package viewerchat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestPreloadViewerDayYAML(t *testing.T) {
	ops := &mockOps{}
	block, err := preloadViewerDayYAML(context.Background(), ops, "nz-4weeks", 20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(block, "day 20") || !strings.Contains(block, "```yaml") || !strings.Contains(block, "trip: T") {
		t.Fatalf("unexpected block: %q", block)
	}
	empty, err := preloadViewerDayYAML(context.Background(), ops, "nz-4weeks", 0)
	if err != nil || empty != "" {
		t.Fatalf("day 0: got %q err=%v", empty, err)
	}
}

func TestToolStatusMessage(t *testing.T) {
	msg := toolStatusMessage([]*schema.FunctionToolCall{
		{Name: "getTrip"},
		{Name: "getTrip"},
		{Name: "getTripYAML"},
	})
	if msg != "using getTrip, getTripYAML" {
		t.Fatalf("got %q", msg)
	}
	if toolStatusMessage(nil) != "using tools" {
		t.Fatal("empty calls")
	}
}

func TestStatusEventJSON(t *testing.T) {
	b, err := json.Marshal(Event{Type: "status", Status: "thinking"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]string
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["type"] != "status" || m["status"] != "thinking" {
		t.Fatalf("unexpected %v", m)
	}
}
