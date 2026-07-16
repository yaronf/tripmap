package itinerary

import "testing"

func TestApplyPatchSwapAndDelete(t *testing.T) {
	trip := Trip{
		Trip: "T",
		Days: []Day{
			{Day: 1, Title: "A", Stops: []Stop{{Name: "a", Lat: 1, Lon: 2}}},
			{Day: 2, Title: "B", Stops: []Stop{{Name: "b", Lat: 3, Lon: 4}}},
			{Day: 3, Title: "C", Stops: []Stop{{Name: "c", Lat: 5, Lon: 6}}},
		},
	}
	if err := ApplyPatch(&trip, Patch{SwapDays: []int{1, 3}}); err != nil {
		t.Fatal(err)
	}
	if trip.Days[0].Title != "C" || trip.Days[0].Day != 1 {
		t.Fatalf("after swap day1 = %+v", trip.Days[0])
	}
	del := 2
	if err := ApplyPatch(&trip, Patch{DeleteDay: &del}); err != nil {
		t.Fatal(err)
	}
	if len(trip.Days) != 2 || trip.Days[1].Day != 2 {
		t.Fatalf("after delete = %+v", trip.Days)
	}
}
