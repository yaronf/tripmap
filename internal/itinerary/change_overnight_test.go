package itinerary

import (
	"strings"
	"testing"
)

func TestApplyChangeOvernightPreservesNextMid(t *testing.T) {
	trip := Trip{
		SchemaVersion: 2,
		Trip:          "T",
		Places: map[string]Place{
			"a": {Title: "A", Lat: -41.0, Lon: 173.0, Type: "overnight"},
			"b": {Title: "B", Lat: -41.1, Lon: 173.1, Type: "overnight"},
			"c": {Title: "C", Lat: -41.2, Lon: 173.2, Type: "via"},
			"d": {Title: "D", Lat: -41.3, Lon: 173.3, Type: "overnight"},
			"e": {Title: "E", Lat: -41.15, Lon: 173.15, Type: "overnight"},
		},
		Days: []Day{
			{Day: 1, Title: "A → B", Route: []Stop{{Place: "a", Type: "overnight"}, {Place: "b", Type: "overnight"}}},
			{Day: 2, Title: "B → D", Route: []Stop{{Place: "b", Type: "overnight"}, {Place: "c", Type: "via"}, {Place: "d", Type: "overnight"}}},
		},
	}
	res, err := ApplyChangeOvernight(&trip, ChangeOvernightArgs{Day: 1, NewEnd: "e", Title: "A → E"})
	if err != nil {
		t.Fatal(err)
	}
	if res.OldEnd != "b" || res.NewEnd != "e" {
		t.Fatalf("%+v", res)
	}
	if trip.Days[0].Route[len(trip.Days[0].Route)-1].Place != "e" {
		t.Fatalf("day1=%+v", trip.Days[0].Route)
	}
	if trip.Days[1].Route[0].Place != "e" {
		t.Fatalf("day2 start=%+v", trip.Days[1].Route)
	}
	if trip.Days[1].Route[1].Place != "c" || trip.Days[1].Route[2].Place != "d" {
		t.Fatalf("day2 mid/end not preserved: %+v", trip.Days[1].Route)
	}
	if !res.PreservedNextMid {
		t.Fatal("expected PreservedNextMid")
	}
}

func TestApplyChangeOvernightRejectsAbsurdDistance(t *testing.T) {
	trip := Trip{
		SchemaVersion: 2,
		Trip:          "T",
		Places: map[string]Place{
			"a": {Title: "A", Lat: -36.8, Lon: 174.7, Type: "overnight"}, // Auckland
			"b": {Title: "B", Lat: -36.9, Lon: 174.8, Type: "overnight"},
			"z": {Title: "Z", Lat: -45.0, Lon: 170.0, Type: "overnight"}, // far south
		},
		Days: []Day{
			{Day: 1, Title: "A → B", Route: []Stop{{Place: "a"}, {Place: "b"}}},
		},
	}
	_, err := ApplyChangeOvernight(&trip, ChangeOvernightArgs{Day: 1, NewEnd: "z"})
	if err == nil || !strings.Contains(err.Error(), "exceeds max") {
		t.Fatalf("err=%v", err)
	}
	_, err = ApplyChangeOvernight(&trip, ChangeOvernightArgs{Day: 1, NewEnd: "z", Force: true})
	if err != nil {
		t.Fatal(err)
	}
}
