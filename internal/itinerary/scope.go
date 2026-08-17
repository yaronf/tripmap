package itinerary

import (
	"fmt"
	"strings"
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
	trip, err := ParseYAML(body)
	if err != nil {
		return nil, err
	}
	byDay := map[int]*Day{}
	for i := range trip.Days {
		byDay[trip.Days[i].Day] = &trip.Days[i]
	}
	start, end := dayNeighborhoodRange(day)
	scoped := Trip{
		SchemaVersion: trip.SchemaVersion,
		Trip:          trip.Trip,
		Description:   trip.Description,
		Start:         trip.Start,
		Places:        map[string]Place{},
		Days:          make([]Day, 0, 3),
	}
	used := map[string]bool{}
	for d := start; d <= end; d++ {
		dayDoc, ok := byDay[d]
		if !ok {
			continue
		}
		scoped.Days = append(scoped.Days, *dayDoc)
		for _, s := range append(append([]Stop{}, dayDoc.Route...), dayDoc.Stops...) {
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
	raw, err := MarshalYAML(scoped)
	if err != nil {
		return nil, err
	}
	header := fmt.Sprintf("# scope: day (days %d..%d; pass scope=full for entire trip)\n", start, end)
	return append([]byte(header), raw...), nil
}
