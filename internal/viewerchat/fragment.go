package viewerchat

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/yaronf/tripmap/internal/itinerary"
)

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
	start := day - 1
	if start < 1 {
		start = 1
	}
	end := day + 1
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

// ContinuityWarnings returns soft warnings when consecutive day route ends/starts mismatch.
func ContinuityWarnings(body []byte, dayNums []int) []string {
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
	if len(dayNums) == 0 {
		for d := range byDay {
			checkPair(d)
		}
		return out
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
