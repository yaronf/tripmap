package itinerary

import (
	"fmt"
	"math"
	"strings"
)

// DefaultMaxOvernightKM rejects absurd single-day overnight legs unless Force is set.
const DefaultMaxOvernightKM = 800.0

// ChangeOvernightArgs is the domain input for a transactional overnight change.
type ChangeOvernightArgs struct {
	Day                int
	NewEnd             string // place id
	Title              string
	AlsoUpdateNextStart bool // default true when unset via pointer; use AlsoUpdateNextStartSet
	AlsoUpdateNextStartSet bool
	Force              bool // bypass distance check
	// NewPlace optionally creates/updates the catalog entry for NewEnd.
	NewPlace *Place
}

// ChangeOvernightResult describes what the domain op did (for tool JSON).
type ChangeOvernightResult struct {
	Day              int
	OldEnd           string
	NewEnd           string
	NextDay          int
	OldNextStart     string
	NewNextStart     string
	PreservedNextMid bool
	DistanceKM       float64
}

// ApplyChangeOvernight mutates trip in place: day N route end → newEnd (overnight),
// and optionally day N+1 route start → newEnd, preserving N+1 mid/end stops.
func ApplyChangeOvernight(t *Trip, args ChangeOvernightArgs) (ChangeOvernightResult, error) {
	if args.Day < 1 {
		return ChangeOvernightResult{}, fmt.Errorf("changeOvernight: day must be >= 1")
	}
	newEnd := strings.TrimSpace(args.NewEnd)
	if newEnd == "" {
		return ChangeOvernightResult{}, fmt.Errorf("changeOvernight: new_end place id is required")
	}
	if args.NewPlace != nil {
		if t.Places == nil {
			t.Places = map[string]Place{}
		}
		p := *args.NewPlace
		if p.Type == "" {
			p.Type = "overnight"
		}
		t.Places[newEnd] = p
	}
	if _, ok := t.Places[newEnd]; !ok {
		return ChangeOvernightResult{}, fmt.Errorf(
			"changeOvernight: unknown place %q — pass places.%s with title/lat/lon/type in the same call, or create it first",
			newEnd, newEnd,
		)
	}

	dayN := findDayPtr(t, args.Day)
	if dayN == nil {
		return ChangeOvernightResult{}, fmt.Errorf("changeOvernight: day %d not found", args.Day)
	}
	if len(dayN.Route) == 0 {
		return ChangeOvernightResult{}, fmt.Errorf("changeOvernight: day %d has no route (cannot set overnight end)", args.Day)
	}

	oldEnd := strings.TrimSpace(dayN.Route[len(dayN.Route)-1].Place)
	startPlace := strings.TrimSpace(dayN.Route[0].Place)

	// Distance: start of day → new end (travel day length proxy).
	dist := placeDistanceKM(t, startPlace, newEnd)
	if !args.Force && dist > DefaultMaxOvernightKM {
		return ChangeOvernightResult{}, fmt.Errorf(
			"changeOvernight: day %d leg ~%.0f km exceeds max %.0f km (%s → %s). "+
				"Use force=true only for ferry/flight days, or pick a nearer overnight. "+
				"For multi-stop route surgery use replaceDayRoutes",
			args.Day, dist, DefaultMaxOvernightKM, startPlace, newEnd,
		)
	}

	// Rewrite end of day N.
	if len(dayN.Route) == 1 {
		dayN.Route[0] = Stop{Place: newEnd, Type: "overnight"}
	} else {
		mid := append([]Stop{}, dayN.Route[:len(dayN.Route)-1]...)
		dayN.Route = append(mid, Stop{Place: newEnd, Type: "overnight"})
	}
	if strings.TrimSpace(args.Title) != "" {
		dayN.Title = strings.TrimSpace(args.Title)
	}

	res := ChangeOvernightResult{
		Day:        args.Day,
		OldEnd:     oldEnd,
		NewEnd:     newEnd,
		DistanceKM: dist,
	}

	updateNext := true
	if args.AlsoUpdateNextStartSet {
		updateNext = args.AlsoUpdateNextStart
	}
	if !updateNext {
		return res, nil
	}

	dayN1 := findDayPtr(t, args.Day+1)
	if dayN1 == nil {
		return res, nil
	}
	res.NextDay = args.Day + 1
	if len(dayN1.Route) == 0 {
		dayN1.Route = []Stop{{Place: newEnd, Type: "overnight"}}
		res.NewNextStart = newEnd
		return res, nil
	}
	res.OldNextStart = strings.TrimSpace(dayN1.Route[0].Place)
	if len(dayN1.Route) == 1 {
		dayN1.Route[0] = Stop{Place: newEnd, Type: "overnight"}
	} else {
		rest := append([]Stop{}, dayN1.Route[1:]...)
		dayN1.Route = append([]Stop{{Place: newEnd, Type: "overnight"}}, rest...)
		res.PreservedNextMid = true
	}
	res.NewNextStart = newEnd
	return res, nil
}

func findDayPtr(t *Trip, day int) *Day {
	for i := range t.Days {
		if t.Days[i].Day == day {
			return &t.Days[i]
		}
	}
	return nil
}

func placeDistanceKM(t *Trip, a, b string) float64 {
	pa, okA := t.Places[a]
	pb, okB := t.Places[b]
	if !okA || !okB || (pa.Lat == 0 && pa.Lon == 0) || (pb.Lat == 0 && pb.Lon == 0) {
		return 0
	}
	return haversineKM(pa.Lat, pa.Lon, pb.Lat, pb.Lon)
}

func haversineKM(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 6371.0
	toRad := math.Pi / 180
	dLat := (lat2 - lat1) * toRad
	dLon := (lon2 - lon1) * toRad
	la1 := lat1 * toRad
	la2 := lat2 * toRad
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(la1)*math.Cos(la2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * r * math.Asin(math.Min(1, math.Sqrt(h)))
}
