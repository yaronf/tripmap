package routebuild

import "github.com/yaronf/tripmap/internal/itinerary"

// MapPoints returns placemark candidates for a day from Stops and Route,
// excluding via points and de-duplicating by location within the day.
func MapPoints(d itinerary.Day) []itinerary.Stop {
	var pts []itinerary.Stop
	seen := map[string]bool{}
	add := func(s itinerary.Stop) {
		if s.Type == "via" {
			return
		}
		key := StopKey(s)
		if seen[key] {
			return
		}
		seen[key] = true
		pts = append(pts, s)
	}
	for _, s := range d.Stops {
		add(s)
	}
	for _, s := range d.Route {
		add(s)
	}
	return pts
}

// ViewerDayStops builds the PWA stop list in day order. The morning lodging
// on a travel day is labeled "depart" (where you wake up), not "overnight"
// (where you sleep). Co-located attraction + overnight are both kept.
func ViewerDayStops(d itinerary.Day) []itinerary.Stop {
	var out []itinerary.Stop
	seen := map[string]bool{}
	add := func(s itinerary.Stop) {
		if s.Type == "via" {
			return
		}
		key := s.Name + "|" + s.Type + "|" + StopKey(s)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, s)
	}

	route := d.Route
	if d.Hike {
		route = EffectiveRoutePoints(d)
	}

	if len(route) >= 2 {
		start, end := route[0], route[len(route)-1]
		startOut := start
		if start.Type == "overnight" && dayHasLaterOvernight(d, start) {
			startOut.Type = "depart"
		}
		add(startOut)
		for i := 1; i < len(route)-1; i++ {
			add(route[i])
		}
		for _, s := range d.Stops {
			add(s)
		}
		add(end)
		return out
	}

	for _, s := range d.Stops {
		add(s)
	}
	for _, s := range d.Route {
		add(s)
	}
	return out
}

func dayHasLaterOvernight(d itinerary.Day, start itinerary.Stop) bool {
	for i, s := range d.Route {
		if i == 0 {
			continue
		}
		if s.Type == "overnight" && !sameStop(s, start) {
			return true
		}
	}
	for _, s := range d.Stops {
		if s.Type == "overnight" && !sameStop(s, start) {
			return true
		}
	}
	return false
}

// EffectiveRoutePoints builds the ordered list of points used to draw lines.
// On hike days, prepends lodging from stops when the route does not already
// start there, and falls back to stops when no route is defined.
func EffectiveRoutePoints(d itinerary.Day) []itinerary.Stop {
	pts := append([]itinerary.Stop{}, d.Route...)
	if len(pts) == 0 && d.Hike {
		for _, s := range d.Stops {
			if s.Type != "attraction" {
				pts = append(pts, s)
			}
		}
	}
	if !d.Hike || len(pts) == 0 {
		return pts
	}
	for _, s := range d.Stops {
		if s.Type == "overnight" && !sameStop(s, pts[0]) {
			return append([]itinerary.Stop{s}, pts...)
		}
	}
	return pts
}

func isTrailPoint(t string) bool {
	switch t {
	case "trailhead", "hut", "viewpoint", "attraction":
		return true
	default:
		return false
	}
}

func isDrivingPoint(t string) bool {
	switch t {
	case "overnight", "ferry_terminal", "airport", "via", "":
		return true
	default:
		return false
	}
}

func isDrivingSegment(a, b itinerary.Stop) bool {
	return (isDrivingPoint(a.Type) && isTrailPoint(b.Type)) ||
		(isTrailPoint(a.Type) && isDrivingPoint(b.Type))
}

func isFerrySegment(a, b itinerary.Stop) bool {
	return a.Type == "ferry_terminal" && b.Type == "ferry_terminal"
}
