package routing

import "math"

// SimplifyGeometry reduces a [lon,lat] polyline with Douglas-Peucker.
// toleranceMeters <= 0 returns the input unchanged.
func SimplifyGeometry(geometry [][]float64, toleranceMeters float64) [][]float64 {
	if toleranceMeters <= 0 || len(geometry) <= 2 {
		out := make([][]float64, len(geometry))
		copy(out, geometry)
		return out
	}
	return douglasPeucker(geometry, toleranceMeters)
}

func douglasPeucker(points [][]float64, tolerance float64) [][]float64 {
	if len(points) <= 2 {
		out := make([][]float64, len(points))
		copy(out, points)
		return out
	}

	maxDist := 0.0
	maxIdx := 0
	end := len(points) - 1
	for i := 1; i < end; i++ {
		d := crossTrackDistanceMeters(points[i], points[0], points[end])
		if d > maxDist {
			maxDist = d
			maxIdx = i
		}
	}

	if maxDist > tolerance {
		left := douglasPeucker(points[:maxIdx+1], tolerance)
		right := douglasPeucker(points[maxIdx:], tolerance)
		return append(left[:len(left)-1], right...)
	}

	return [][]float64{points[0], points[end]}
}

func crossTrackDistanceMeters(p, a, b []float64) float64 {
	if len(p) < 2 || len(a) < 2 || len(b) < 2 {
		return 0
	}
	if a[0] == b[0] && a[1] == b[1] {
		return haversineMeters(a[1], a[0], p[1], p[0])
	}

	const earthRadius = 6371000
	lat1 := degToRad(a[1])
	lon1 := degToRad(a[0])
	lat2 := degToRad(b[1])
	lon2 := degToRad(b[0])
	lat3 := degToRad(p[1])
	lon3 := degToRad(p[0])

	d13 := angularDistance(lat1, lon1, lat3, lon3)
	brng13 := bearing(lat1, lon1, lat3, lon3)
	brng12 := bearing(lat1, lon1, lat2, lon2)

	return math.Abs(math.Asin(math.Sin(d13)*math.Sin(brng13-brng12))) * earthRadius
}

func haversineMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadius = 6371000
	dLat := degToRad(lat2 - lat1)
	dLon := degToRad(lon2 - lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(degToRad(lat1))*math.Cos(degToRad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * earthRadius * math.Asin(math.Sqrt(a))
}

func angularDistance(lat1, lon1, lat2, lon2 float64) float64 {
	dLat := lat2 - lat1
	dLon := lon2 - lon1
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1)*math.Cos(lat2)*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * math.Asin(math.Sqrt(a))
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	dLon := lon2 - lon1
	y := math.Sin(dLon) * math.Cos(lat2)
	x := math.Cos(lat1)*math.Sin(lat2) - math.Sin(lat1)*math.Cos(lat2)*math.Cos(dLon)
	return math.Atan2(y, x)
}

func degToRad(deg float64) float64 {
	return deg * math.Pi / 180
}
