package viewerchat

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

const maxPlaceAwayFromDayKM = 180.0 // reject wrong-city pins (Wellington≈240km from Waimarino)

// rejectWeakNewPlaces catches Avis-class failures: missing Google maps_url, or
// lat/lon in the wrong city relative to the day being edited.
func rejectWeakNewPlaces(patchJSON []byte, beforeYAML []byte) error {
	var p itinerary.Patch
	if err := json.Unmarshal(patchJSON, &p); err != nil || len(p.Places) == 0 {
		return nil
	}
	trip, err := itinerary.ParseYAML(beforeYAML)
	if err != nil {
		return nil
	}
	dayNum := 0
	if p.UpsertStop != nil {
		dayNum = p.UpsertStop.Day
	}
	for key := range p.Days {
		if n, err := strconv.Atoi(strings.TrimSpace(key)); err == nil && n > 0 {
			dayNum = n
			break
		}
	}

	for id, raw := range p.Places {
		if _, exists := trip.Places[id]; exists {
			continue // updates to existing places — skip strict create checks
		}
		m, ok := asObjectMap(raw)
		if !ok {
			return fmt.Errorf("new place %q must be an object with title/lat/lon/type/maps_url", id)
		}
		lat, lon, okLL := placeLatLon(m)
		if !okLL {
			return fmt.Errorf(
				"new place %q needs top-level lat and lon (map pin). "+
					"Use coordinates for the correct city on that day — not a guessed centroid in another city",
				id,
			)
		}
		mapsURL, _ := m["maps_url"].(string)
		mapsURL = strings.TrimSpace(mapsURL)
		if mapsURL == "" || !looksLikeGoogleMapsURL(mapsURL) {
			return fmt.Errorf(
				"new place %q needs places.%s.maps_url set to a Google Maps URL "+
					"(include the city in the query, e.g. Avis+Auckland or Avis+Wellington). "+
					"Leaflet pins use lat/lon; maps_url is the timeline Google link",
				id, id,
			)
		}
		if dayNum < 1 {
			continue
		}
		d, ok := dayByNumber(trip, dayNum)
		if !ok || len(d.Route) == 0 {
			continue
		}
		if minKMToDayRoute(trip, d, lat, lon) > maxPlaceAwayFromDayKM {
			return fmt.Errorf(
				"new place %q lat/lon looks far from day %d's route (>%g km from every route place). "+
					"Fix coordinates to the correct city for that day before writing "+
					"(Auckland pick-up must not use Wellington coords)",
				id, dayNum, maxPlaceAwayFromDayKM,
			)
		}
	}
	return nil
}

func asObjectMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func placeLatLon(m map[string]any) (lat, lon float64, ok bool) {
	lat, ok1 := asFloat(m["lat"])
	lon, ok2 := asFloat(m["lon"])
	return lat, lon, ok1 && ok2
}

func asFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func looksLikeGoogleMapsURL(u string) bool {
	lower := strings.ToLower(u)
	return strings.Contains(lower, "google.com/maps") ||
		strings.Contains(lower, "maps.google.") ||
		strings.Contains(lower, "goo.gl/maps") ||
		strings.HasPrefix(lower, "https://g.page/") ||
		strings.HasPrefix(lower, "http://g.page/")
}

func minKMToDayRoute(trip itinerary.Trip, d itinerary.Day, lat, lon float64) float64 {
	min := math.MaxFloat64
	for _, s := range d.Route {
		p, ok := trip.Places[strings.TrimSpace(s.Place)]
		if !ok || (p.Lat == 0 && p.Lon == 0) {
			continue
		}
		km := haversineKMLocal(lat, lon, p.Lat, p.Lon)
		if km < min {
			min = km
		}
	}
	if min == math.MaxFloat64 {
		return 0
	}
	return min
}

func haversineKMLocal(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	toRad := math.Pi / 180
	dLat := (lat2 - lat1) * toRad
	dLon := (lon2 - lon1) * toRad
	la1 := lat1 * toRad
	la2 := lat2 * toRad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(h)))
}
