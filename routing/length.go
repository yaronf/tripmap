package routing

// PathLengthMeters sums great-circle lengths along a [lon,lat] polyline.
func PathLengthMeters(geometry [][]float64) float64 {
	if len(geometry) < 2 {
		return 0
	}
	var sum float64
	for i := 1; i < len(geometry); i++ {
		a, b := geometry[i-1], geometry[i]
		if len(a) < 2 || len(b) < 2 {
			continue
		}
		sum += haversineMeters(a[1], a[0], b[1], b[0])
	}
	return sum
}
