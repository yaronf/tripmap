package viewerchat

import (
	"strings"
	"testing"
)

func TestEmbeddedPrompt(t *testing.T) {
	p := baseSystemPrompt()
	if !strings.Contains(p, "tripmap's itinerary assistant") {
		t.Fatalf("prompt missing identity: %q", p)
	}
	if strings.Contains(p, "Trip context") {
		t.Fatal("static prompt should not include dynamic trip context")
	}
}

func TestChatToolsFromOpenAPI(t *testing.T) {
	tools := chatTools()
	if len(tools) < 2 {
		t.Fatalf("expected web_search + functions, got %d", len(tools))
	}
	if tools[0].OfWebSearch == nil {
		t.Fatal("first tool should be web_search")
	}
	names := map[string]bool{}
	for _, tool := range tools[1:] {
		if tool.OfFunction == nil {
			t.Fatalf("expected function tool, got %#v", tool)
		}
		names[tool.OfFunction.Name] = true
		if tool.OfFunction.Parameters == nil {
			t.Fatalf("%s missing parameters", tool.OfFunction.Name)
		}
	}
	for _, want := range []string{
		"get_trip_summary", "get_schema", "get_trip_yaml",
		"get_day", "set_day_photo", "patch_trip",
	} {
		if !names[want] {
			t.Fatalf("missing tool %s in %#v", want, names)
		}
	}
}
