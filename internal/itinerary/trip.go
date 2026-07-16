package itinerary

// Stop is a single point in a day. Type controls whether it appears on the map,
// on the route line, or both:
//
//	""              generic waypoint: map placemark + route point (default)
//	overnight       lodging: map placemark + route endpoint
//	hut             backcountry hut: map placemark + hike route point
//	via             route-shaping only: on the line, no placemark
//	attraction      map placemark only, not on the route
//	viewpoint       map placemark only, not on the route
//	trailhead       trail car park: map placemark + route point
//	ferry_terminal  map placemark + route point (draw with ferry styling)
//	airport         airport: map placemark + route point
type Stop struct {
	Name string  `yaml:"name" json:"name"`
	Lat  float64 `yaml:"lat" json:"lat"`
	Lon  float64 `yaml:"lon" json:"lon"`
	Type string  `yaml:"type,omitempty" json:"type,omitempty"`
	Notes string `yaml:"notes,omitempty" json:"notes,omitempty"`
	Photo string `yaml:"photo,omitempty" json:"photo,omitempty"`
	// PhotoCaption is optional hover / lightbox text for Photo.
	PhotoCaption string `yaml:"photo_caption,omitempty" json:"photo_caption,omitempty"`
}

// Day is one day of the trip. Route explicitly defines the line, while Stops
// defines additional map placemarks. Hike days may combine OSRM driving
// approaches with straight-line trail segments.
type Day struct {
	Day          int    `yaml:"day" json:"day"`
	Date         string `yaml:"date,omitempty" json:"date,omitempty"` // YYYY-MM-DD; optional, or derived from trip.start
	Title        string `yaml:"title" json:"title"`
	Route        []Stop `yaml:"route,omitempty" json:"route,omitempty"`
	Stops        []Stop `yaml:"stops,omitempty" json:"stops,omitempty"`
	Notes        string `yaml:"notes,omitempty" json:"notes,omitempty"`
	Photo        string `yaml:"photo,omitempty" json:"photo,omitempty"`
	PhotoCaption string `yaml:"photo_caption,omitempty" json:"photo_caption,omitempty"`
	Hike         bool   `yaml:"hike,omitempty" json:"hike,omitempty"`
	Ferry        bool   `yaml:"ferry,omitempty" json:"ferry,omitempty"`
}

// Trip is the YAML itinerary document.
type Trip struct {
	SchemaVersion int    `yaml:"schema_version,omitempty" json:"schema_version,omitempty"`
	Trip          string `yaml:"trip" json:"trip"`
	Description   string `yaml:"description,omitempty" json:"description,omitempty"`
	// Start is an optional trip start date (YYYY-MM-DD). When set, days
	// without an explicit date get start + (day number − 1).
	Start string `yaml:"start,omitempty" json:"start,omitempty"`
	Days  []Day  `yaml:"days" json:"days"`
}
