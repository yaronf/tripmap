package viewerchat

import (
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"
)

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
