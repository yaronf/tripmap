package tripops

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/yaronf/tripmap/internal/store"
)

func TestBuildSummaryIncludesBundleDriveStats(t *testing.T) {
	st := store.NewMem()
	const tripID = "nz"
	yaml := []byte(`schema_version: 2
trip: NZ
days:
  - day: 1
    title: Day one
    route: []
  - day: 5
    title: Long drive
    route: []
`)
	if _, err := st.PutYAML(context.Background(), tripID, yaml); err != nil {
		t.Fatal(err)
	}
	tj, _ := json.Marshal(map[string]any{
		"id":    tripID,
		"title": "NZ",
		"units": "km",
		"days": []map[string]any{
			{"day": 1, "title": "Day one", "drive_dist": 12.3, "drive_min": 18},
			{"day": 5, "title": "Long drive", "drive_dist": 210.5, "drive_min": 165},
		},
	})
	st.PutBundleFile(tripID, "trip.json", tj)

	sum, err := BuildSummary(context.Background(), st, tripID)
	if err != nil {
		t.Fatal(err)
	}
	if sum.Units != "km" {
		t.Fatalf("units = %q", sum.Units)
	}
	if len(sum.DayStats) != 2 {
		t.Fatalf("day_stats = %+v", sum.DayStats)
	}
	if sum.DayStats[1].Day != 5 || sum.DayStats[1].DriveMin != 165 || sum.DayStats[1].DriveDist != 210.5 {
		t.Fatalf("day 5 stats = %+v", sum.DayStats[1])
	}
}

func TestBuildSummaryWithoutBundle(t *testing.T) {
	st := store.NewMem()
	const tripID = "bare"
	yaml := []byte(`schema_version: 2
trip: Bare
days:
  - day: 1
    title: One
    route: []
`)
	if _, err := st.PutYAML(context.Background(), tripID, yaml); err != nil {
		t.Fatal(err)
	}
	sum, err := BuildSummary(context.Background(), st, tripID)
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.DayStats) != 0 {
		t.Fatalf("expected no day_stats without bundle, got %+v", sum.DayStats)
	}
}
