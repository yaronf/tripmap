package itinerary

import "testing"

func TestParseReplaceDayRoutesRequiresRoute(t *testing.T) {
	_, err := ParseReplaceDayRoutes([]byte(`{"days":{"7":{"title":"X"}}}`))
	if err == nil {
		t.Fatal("expected error without route")
	}
	_, err = ParseReplaceDayRoutes([]byte(`{"days":{"7":{"title":"X","route":[]}}}`))
	if err == nil {
		t.Fatal("expected error for empty route")
	}
}

func TestParseReplaceDayRoutesNormalizesViaEnds(t *testing.T) {
	p, err := ParseReplaceDayRoutes([]byte(`{
		"days":{"10":{"route":[
			{"place":"murchison","type":"via","maps_url":"https://maps/?q=wrong"},
			{"place":"westport","type":"via"},
			{"place":"punakaiki","type":"overnight"}
		]}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	day := p.Days["10"].(map[string]any)
	route := day["route"].([]any)
	start := route[0].(map[string]any)
	if start["type"] != "overnight" {
		t.Fatalf("start type = %v, want overnight", start["type"])
	}
	if _, ok := start["maps_url"]; ok {
		t.Fatal("expected maps_url cleared on rewritten start")
	}
	mid := route[1].(map[string]any)
	if mid["type"] != "via" {
		t.Fatalf("mid type = %v, want via", mid["type"])
	}
}

func TestParseReplaceDayRoutesKeepsFerryStart(t *testing.T) {
	p, err := ParseReplaceDayRoutes([]byte(`{
		"days":{"7":{"route":[
			{"place":"wellington-ferry-terminal","type":"ferry_terminal"},
			{"place":"picton-ferry-terminal","type":"ferry_terminal"},
			{"place":"motueka","type":"overnight"}
		]}}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	start := p.Days["7"].(map[string]any)["route"].([]any)[0].(map[string]any)
	if start["type"] != "ferry_terminal" {
		t.Fatalf("ferry start rewritten: %v", start["type"])
	}
}

func TestApplyReplaceDayRoutes(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"a": {Title: "A", Lat: 1, Lon: 2, Type: "overnight"},
			"b": {Title: "B", Lat: 3, Lon: 4, Type: "overnight"},
			"c": {Title: "C", Lat: 5, Lon: 6, Type: "overnight"},
		},
		Days: []Day{
			{Day: 1, Title: "A → B", Route: []Stop{{Place: "a"}, {Place: "b"}}},
			{Day: 2, Title: "B → C", Route: []Stop{{Place: "b"}, {Place: "c"}}},
		},
	}
	p, err := ParseReplaceDayRoutes([]byte(`{
		"places":{"d":{"title":"D","lat":7,"lon":8,"type":"overnight"}},
		"days":{
			"1":{"title":"A → D","route":[{"place":"a"},{"place":"d"}]},
			"2":{"title":"D → C","route":[{"place":"d"},{"place":"c"}]}
		}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := ApplyPatch(&trip, p); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Title != "A → D" || trip.Days[0].Route[1].Place != "d" {
		t.Fatalf("day1 = %+v", trip.Days[0])
	}
	if trip.Days[1].Route[0].Place != "d" {
		t.Fatalf("day2 = %+v", trip.Days[1])
	}
}
