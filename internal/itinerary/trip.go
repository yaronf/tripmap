package itinerary

// Place is a reusable catalog entry (coords, default type, optional enrichment).
type Place struct {
	Title        string     `yaml:"title" json:"title"`
	Lat          float64    `yaml:"lat" json:"lat"`
	Lon          float64    `yaml:"lon" json:"lon"`
	Type         string     `yaml:"type,omitempty" json:"type,omitempty"`
	Notes        string     `yaml:"notes,omitempty" json:"notes,omitempty"`
	Photo        string     `yaml:"photo,omitempty" json:"photo,omitempty"`
	PhotoCaption string     `yaml:"photo_caption,omitempty" json:"photo_caption,omitempty"`
	// MapsURL overrides the viewer pin destination (Google Maps place URL).
	// When empty, the pin opens a lat/lon search.
	MapsURL string     `yaml:"maps_url,omitempty" json:"maps_url,omitempty"`
	Info    *PlaceInfo `yaml:"info,omitempty" json:"info,omitempty"`
}

// PlaceInfo is structured enrichment (typically AI-generated). Optional throughout.
type PlaceInfo struct {
	Source     *PlaceSource      `yaml:"source,omitempty" json:"source,omitempty"`
	Links      []PlaceLink       `yaml:"links,omitempty" json:"links,omitempty"`
	Stats      *PlaceStats       `yaml:"stats,omitempty" json:"stats,omitempty"`
	Logistics  *PlaceLogistics   `yaml:"logistics,omitempty" json:"logistics,omitempty"`
	Facilities *PlaceFacilities  `yaml:"facilities,omitempty" json:"facilities,omitempty"`
	Warnings   []string          `yaml:"warnings,omitempty" json:"warnings,omitempty"`
	Highlights []string          `yaml:"highlights,omitempty" json:"highlights,omitempty"`
}

// PlaceSource is light provenance for regenerated enrichment.
type PlaceSource struct {
	GeneratedBy string `yaml:"generated_by,omitempty" json:"generated_by,omitempty"`
	GeneratedAt string `yaml:"generated_at,omitempty" json:"generated_at,omitempty"`
}

// PlaceLink is a typed URL (alltrails, doc, maps, …).
type PlaceLink struct {
	Type  string `yaml:"type" json:"type"`
	Title string `yaml:"title,omitempty" json:"title,omitempty"`
	URL   string `yaml:"url" json:"url"`
}

// PlaceStats holds optional trail/drive statistics.
type PlaceStats struct {
	DistanceKm *float64 `yaml:"distance_km,omitempty" json:"distance_km,omitempty"`
	Duration   string   `yaml:"duration,omitempty" json:"duration,omitempty"`
	AscentM    *float64 `yaml:"ascent_m,omitempty" json:"ascent_m,omitempty"`
	Difficulty string   `yaml:"difficulty,omitempty" json:"difficulty,omitempty"`
}

// PlaceLogistics holds optional practical logistics.
type PlaceLogistics struct {
	Parking         string `yaml:"parking,omitempty" json:"parking,omitempty"`
	BookingRequired *bool  `yaml:"booking_required,omitempty" json:"booking_required,omitempty"`
}

// PlaceFacilities holds optional amenity flags.
type PlaceFacilities struct {
	Toilets        *bool `yaml:"toilets,omitempty" json:"toilets,omitempty"`
	DrinkingWater  *bool `yaml:"drinking_water,omitempty" json:"drinking_water,omitempty"`
}

// Stop is a day-local reference to a place. Type/Notes/Photo/MapsURL override the catalog.
// Name/Lat/Lon/Info are hydrated by ResolvePlaces and are not written to YAML.
type Stop struct {
	Place        string `yaml:"place" json:"place"`
	Type         string `yaml:"type,omitempty" json:"type,omitempty"`
	Notes        string `yaml:"notes,omitempty" json:"notes,omitempty"`
	Photo        string `yaml:"photo,omitempty" json:"photo,omitempty"`
	PhotoCaption string `yaml:"photo_caption,omitempty" json:"photo_caption,omitempty"`
	MapsURL      string `yaml:"maps_url,omitempty" json:"maps_url,omitempty"`

	Name string     `yaml:"-" json:"-"`
	Lat  float64    `yaml:"-" json:"-"`
	Lon  float64    `yaml:"-" json:"-"`
	Info *PlaceInfo `yaml:"-" json:"-"`
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
	SchemaVersion int              `yaml:"schema_version,omitempty" json:"schema_version,omitempty"`
	Trip          string           `yaml:"trip" json:"trip"`
	Description   string           `yaml:"description,omitempty" json:"description,omitempty"`
	// Start is an optional trip start date (YYYY-MM-DD). When set, days
	// without an explicit date get start + (day number − 1).
	Start  string           `yaml:"start,omitempty" json:"start,omitempty"`
	Places map[string]Place `yaml:"places,omitempty" json:"places,omitempty"`
	Days   []Day            `yaml:"days" json:"days"`
}
