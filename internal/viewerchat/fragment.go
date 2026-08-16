package viewerchat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

// dayNeighborhoodRange returns inclusive day numbers max(1, day-1)..day+1.
func dayNeighborhoodRange(day int) (start, end int) {
	start = day - 1
	if start < 1 {
		start = 1
	}
	return start, day + 1
}

// BuildDayScopedYAML returns a YAML document with only days in the viewer
// neighborhood (day±1) and places referenced by those days' route/stops.
func BuildDayScopedYAML(body []byte, day int) ([]byte, error) {
	if day < 1 {
		return nil, fmt.Errorf("day must be >= 1")
	}
	trip, err := itinerary.ParseYAML(body)
	if err != nil {
		return nil, err
	}
	byDay := map[int]*itinerary.Day{}
	for i := range trip.Days {
		byDay[trip.Days[i].Day] = &trip.Days[i]
	}
	start, end := dayNeighborhoodRange(day)
	scoped := itinerary.Trip{
		SchemaVersion: trip.SchemaVersion,
		Trip:          trip.Trip,
		Description:   trip.Description,
		Start:         trip.Start,
		Places:        map[string]itinerary.Place{},
		Days:          make([]itinerary.Day, 0, 3),
	}
	used := map[string]bool{}
	for d := start; d <= end; d++ {
		dayDoc, ok := byDay[d]
		if !ok {
			continue
		}
		scoped.Days = append(scoped.Days, *dayDoc)
		for _, s := range append(append([]itinerary.Stop{}, dayDoc.Route...), dayDoc.Stops...) {
			id := strings.TrimSpace(s.Place)
			if id == "" || used[id] {
				continue
			}
			used[id] = true
			if p, ok := trip.Places[id]; ok {
				scoped.Places[id] = p
			}
		}
	}
	if len(scoped.Days) == 0 {
		return nil, fmt.Errorf("day %d not found", day)
	}
	raw, err := itinerary.MarshalYAML(scoped)
	if err != nil {
		return nil, err
	}
	header := fmt.Sprintf("# scope: day (days %d..%d; pass scope=full for entire trip)\n", start, end)
	return append([]byte(header), raw...), nil
}

// TripFragmentStop is one route stop in the per-turn orientation sketch.
type TripFragmentStop struct {
	Place string `json:"place"`
	Type  string `json:"type,omitempty"`
}

// TripFragmentDay is one day in trip_fragment.
type TripFragmentDay struct {
	Day   int                `json:"day"`
	Title string             `json:"title,omitempty"`
	Route []TripFragmentStop `json:"route"`
}

// TripFragment is compact day N±1 orientation injected into Instructions.
type TripFragment struct {
	Days []TripFragmentDay `json:"days"`
}

// BuildTripFragment returns days from max(1, day-1) through day+1 inclusive.
func BuildTripFragment(body []byte, day int) (TripFragment, error) {
	if day < 1 {
		return TripFragment{}, fmt.Errorf("day must be >= 1")
	}
	trip, err := itinerary.ParseYAML(body)
	if err != nil {
		return TripFragment{}, err
	}
	byDay := map[int]*itinerary.Day{}
	for i := range trip.Days {
		byDay[trip.Days[i].Day] = &trip.Days[i]
	}
	start, end := dayNeighborhoodRange(day)
	out := TripFragment{Days: make([]TripFragmentDay, 0, 3)}
	for d := start; d <= end; d++ {
		dayDoc, ok := byDay[d]
		if !ok {
			continue
		}
		fd := TripFragmentDay{Day: d, Title: dayDoc.Title, Route: make([]TripFragmentStop, 0, len(dayDoc.Route))}
		for _, s := range dayDoc.Route {
			typ := s.Type
			if typ == "" {
				typ = trip.Places[s.Place].Type
			}
			fd.Route = append(fd.Route, TripFragmentStop{Place: s.Place, Type: typ})
		}
		out.Days = append(out.Days, fd)
	}
	return out, nil
}

// ContinuityWarnings returns soft warnings when consecutive day route ends/starts mismatch
// for the given days (and each day's predecessor pair). Empty dayNums means no check —
// callers must pass touched/viewer days; never use empty as "scan whole trip".
func ContinuityWarnings(body []byte, dayNums []int) []string {
	if len(dayNums) == 0 {
		return nil
	}
	trip, err := itinerary.ParseYAML(body)
	if err != nil {
		return nil
	}
	byDay := map[int]*itinerary.Day{}
	for i := range trip.Days {
		byDay[trip.Days[i].Day] = &trip.Days[i]
	}
	seen := map[string]bool{}
	var out []string
	checkPair := func(n int) {
		a, okA := byDay[n]
		b, okB := byDay[n+1]
		if !okA || !okB || len(a.Route) == 0 || len(b.Route) == 0 {
			return
		}
		end := strings.TrimSpace(a.Route[len(a.Route)-1].Place)
		start := strings.TrimSpace(b.Route[0].Place)
		if end == "" || start == "" || end == start {
			return
		}
		key := fmt.Sprintf("%d:%s->%d:%s", n, end, n+1, start)
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, fmt.Sprintf(
			"day %d route end %q != day %d route start %q (informational; multi-step edits may be temporarily inconsistent)",
			n, end, n+1, start,
		))
	}
	for _, d := range dayNums {
		checkPair(d)
		checkPair(d - 1)
	}
	return out
}

func dayNumsFromReplaceArgs(argsJSON string) []int {
	p, err := itinerary.ParseReplaceDayRoutes([]byte(argsJSON))
	if err != nil {
		return nil
	}
	out := make([]int, 0, len(p.Days))
	for k := range p.Days {
		n, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil || n < 1 {
			continue
		}
		out = append(out, n)
	}
	return out
}
