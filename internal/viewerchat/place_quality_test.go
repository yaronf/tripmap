package viewerchat

import (
	"strings"
	"testing"
)

func TestRejectWeakNewPlacesWrongCity(t *testing.T) {
	yaml := []byte(`
trip: T
schema_version: 2
places:
  auckland: {title: Auckland, type: overnight, lat: -36.85, lon: 174.76}
  waimarino: {title: Waimarino, type: overnight, lat: -39.16, lon: 175.4}
days:
  - day: 3
    title: Drive
    route: [{place: auckland, type: overnight}, {place: waimarino, type: overnight}]
`)
	// Wellington coords for an Auckland-day place — Avis failure mode.
	err := rejectWeakNewPlaces([]byte(`{"places":{"avis-cbd":{"title":"Avis CBD","lat":-41.29,"lon":174.78,"type":"rental","maps_url":"https://www.google.com/maps/search/?api=1&query=Avis+Wellington"}},"upsert_stop":{"day":3,"list":"stops","place":"avis-cbd"}}`), yaml)
	if err == nil || !strings.Contains(err.Error(), "far from day 3") {
		t.Fatalf("err=%v", err)
	}
	err = rejectWeakNewPlaces([]byte(`{"places":{"avis-auckland":{"title":"Avis Auckland","lat":-36.85,"lon":174.76,"type":"rental"}},"upsert_stop":{"day":3,"list":"stops","place":"avis-auckland"}}`), yaml)
	if err == nil || !strings.Contains(err.Error(), "maps_url") {
		t.Fatalf("missing maps_url err=%v", err)
	}
	err = rejectWeakNewPlaces([]byte(`{"places":{"avis-auckland":{"title":"Avis Auckland","lat":-36.8483,"lon":174.7554,"type":"rental","maps_url":"https://www.google.com/maps/search/?api=1&query=Avis+Auckland"}},"upsert_stop":{"day":3,"list":"stops","place":"avis-auckland"}}`), yaml)
	if err != nil {
		t.Fatal(err)
	}
}
