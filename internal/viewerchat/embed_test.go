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
	if !strings.Contains(p, "savePreference") {
		t.Fatal("prompt should mention savePreference")
	}
	if !strings.Contains(p, "saveLearning") {
		t.Fatal("prompt should mention saveLearning")
	}
	if !strings.Contains(p, "logistics.opening_hours") {
		t.Fatal("prompt should direct opening hours to logistics.opening_hours")
	}
	if !strings.Contains(p, "follow-up getTripYAML confirms") {
		t.Fatal("prompt should require verify-after for note/pin claims")
	}
	if !strings.Contains(p, "Itinerary integrity") {
		t.Fatal("prompt should mention itinerary integrity")
	}
	if !strings.Contains(p, "Never use swap_days to change") {
		t.Fatal("prompt should forbid swap_days for overnight/endpoint changes")
	}
	if !strings.Contains(p, "replaceDayRoutes") {
		t.Fatal("prompt should mention replaceDayRoutes")
	}
	if !strings.Contains(p, "first non-latest") {
		t.Fatal("prompt should explain undo via non-latest version")
	}
	if !strings.Contains(p, `type "overnight"`) && !strings.Contains(p, `type \"overnight\"`) {
		if !strings.Contains(p, "never \"via\"") && !strings.Contains(p, `never "via"`) {
			t.Fatal("prompt should require overnight day-role types on lodging ends")
		}
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
		"listVersions", "getVersion", "restoreVersion", "patchTrip", "replaceDayRoutes",
		"listPreferences", "savePreference", "forgetPreference",
		"listLearnings", "saveLearning", "forgetLearning",
	} {
		if !names[want] {
			t.Fatalf("missing tool %s in %#v", want, names)
		}
	}
	for _, tool := range tools[1:] {
		if tool.OfFunction.Name != "forgetPreference" {
			continue
		}
		props, _ := tool.OfFunction.Parameters["properties"].(map[string]any)
		if _, ok := props["preference_id"]; !ok {
			t.Fatalf("forgetPreference should expose preference_id: %#v", props)
		}
	}
	for _, tool := range tools[1:] {
		if tool.OfFunction.Name != "forgetLearning" {
			continue
		}
		props, _ := tool.OfFunction.Parameters["properties"].(map[string]any)
		if _, ok := props["learning_id"]; !ok {
			t.Fatalf("forgetLearning should expose learning_id: %#v", props)
		}
	}
	for _, ban := range []string{"listTrips", "createTrip", "putTripYAML", "get_day", "get_trip_summary"} {
		if names[ban] {
			t.Fatalf("unexpected tool %s in chat audience", ban)
		}
	}
}
