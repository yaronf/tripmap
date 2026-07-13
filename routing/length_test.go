package routing

import "testing"

func TestPathLengthMeters(t *testing.T) {
	// ~111 km per degree latitude
	geom := [][]float64{{0, 0}, {0, 1}}
	got := PathLengthMeters(geom)
	if got < 110000 || got > 112000 {
		t.Fatalf("PathLengthMeters = %v, want ~111 km", got)
	}
	if PathLengthMeters(nil) != 0 || PathLengthMeters(geom[:1]) != 0 {
		t.Fatal("short paths should be 0")
	}
}
