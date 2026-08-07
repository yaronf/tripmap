package itinerary

import "testing"

func TestResolvePlacesMapsURL(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Places: map[string]Place{
			"from-catalog": {
				Title:   "Catalog Place",
				Lat:     1,
				Lon:     2,
				MapsURL: "https://maps.google.com/?q=catalog",
			},
			"overridden": {
				Title:   "Override Place",
				Lat:     3,
				Lon:     4,
				MapsURL: "https://maps.google.com/?q=catalog-default",
			},
			"coords-only": {
				Title: "Coords Only",
				Lat:   5,
				Lon:   6,
			},
		},
		Days: []Day{{
			Day: 1,
			Stops: []Stop{
				{Place: "from-catalog"},
				{Place: "overridden", MapsURL: "https://maps.google.com/?q=day-override"},
				{Place: "coords-only"},
			},
		}},
	}
	if err := ResolvePlaces(&trip); err != nil {
		t.Fatal(err)
	}
	s := trip.Days[0].Stops
	if s[0].MapsURL != "https://maps.google.com/?q=catalog" {
		t.Fatalf("catalog inherit: got %q", s[0].MapsURL)
	}
	if s[1].MapsURL != "https://maps.google.com/?q=day-override" {
		t.Fatalf("day override: got %q", s[1].MapsURL)
	}
	if s[2].MapsURL != "" {
		t.Fatalf("coords-only should have empty maps_url, got %q", s[2].MapsURL)
	}
}
