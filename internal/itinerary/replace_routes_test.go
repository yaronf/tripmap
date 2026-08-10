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
