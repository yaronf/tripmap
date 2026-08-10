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
	if !strings.Contains(p, "getVersion") {
		t.Fatal("prompt should mention getVersion for history inspection")
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
		name := tool.OfFunction.Name
		names[name] = true
		if tool.OfFunction.Parameters == nil {
			t.Fatalf("%s missing parameters", name)
		}
		props, _ := tool.OfFunction.Parameters["properties"].(map[string]any)
		if _, ok := props["id"]; ok {
			t.Fatalf("%s should not expose session path param id", name)
		}
		if _, ok := props["Idempotency-Key"]; ok {
			t.Fatalf("%s should not expose Idempotency-Key", name)
		}
	}
	for _, want := range []string{
		"getSchema", "getTrip", "getTripYAML", "setDayPhoto",
		"listVersions", "getVersion", "restoreVersion", "patchTrip",
	} {
		if !names[want] {
			t.Fatalf("missing tool %s in %#v", want, names)
		}
	}
	for _, ban := range []string{"listTrips", "createTrip", "putTripYAML", "get_day", "get_trip_summary"} {
		if names[ban] {
			t.Fatalf("unexpected tool %s in chat audience", ban)
		}
	}
}
