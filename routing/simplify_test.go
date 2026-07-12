package routing

import "testing"

func TestSimplifyGeometryPreservesEndpoints(t *testing.T) {
	geom := [][]float64{
		{0, 0},
		{0.001, 0.001},
		{0.002, 0.002},
		{0.003, 0.003},
		{0.004, 0.004},
	}
	out := SimplifyGeometry(geom, 500)
	if len(out) < 2 {
		t.Fatalf("simplified = %d points, want at least 2", len(out))
	}
	if out[0][0] != geom[0][0] || out[0][1] != geom[0][1] {
		t.Fatalf("start = %v, want %v", out[0], geom[0])
	}
	last := out[len(out)-1]
	end := geom[len(geom)-1]
	if last[0] != end[0] || last[1] != end[1] {
		t.Fatalf("end = %v, want %v", last, end)
	}
}

func TestSimplifyGeometryZeroTolerance(t *testing.T) {
	geom := [][]float64{{1, 2}, {3, 4}, {5, 6}}
	out := SimplifyGeometry(geom, 0)
	if len(out) != len(geom) {
		t.Fatalf("len = %d, want %d", len(out), len(geom))
	}
}

func TestSimplifyGeometryReducesPoints(t *testing.T) {
	var geom [][]float64
	for i := 0; i <= 100; i++ {
		geom = append(geom, []float64{float64(i) * 0.0001, 0})
	}
	out := SimplifyGeometry(geom, 50)
	if len(out) >= len(geom) {
		t.Fatalf("simplified len = %d, want fewer than %d", len(out), len(geom))
	}
}
