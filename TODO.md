# Roadmap

## Planned

### Schema
- [ ] Places registry: define `places:` once, reference by ID in route/stops
- [ ] Stop priority flags: `optional`, `backup`, `must`
- [ ] First-class overnight block (place, nights, notes)
- [ ] Booking metadata on stops (required, opens, status) in KML descriptions
- [ ] Weather backup hints on hike days (`swap_with`) in descriptions
- [ ] Trip dates in YAML (`start`, `end`) for export and descriptions

### Routing
- [ ] GraphHopper backend
- [ ] Valhalla backend
- [ ] Offline routing support

### KML
- [ ] Daily colors
- [ ] Driving distance/time
- [ ] Rich HTML descriptions

### Data
- [ ] Geocoding

### Export
- [ ] GPX
- [ ] GeoJSON
- [ ] PDF itinerary
- [ ] Google Maps links

### CLI
- [ ] Config file
- [ ] Verbose logging
- [ ] Cache routing responses
- [ ] Validate itinerary

## Completed

### Routing
- [x] OSRM backend
- [x] Explicitly separate `route` waypoints from `stops` placemarks
- [x] Support route-only `via` waypoints
- [x] Mixed drive/hike/ferry segments on the same day
- [x] Route simplification (`--simplify`, `--mymaps`)

### KML
- [x] Typed stop icons
- [x] Ferry styling
- [x] Hike styling
- [x] Trip, day, and stop descriptions
- [x] Global placemark dedup by location
- [x] Google My Maps output (flatten folders, split long lines)

### Data
- [x] Typed stops: `overnight`, `hut`, `via`, `attraction`, `viewpoint`, `trailhead`, `ferry_terminal`, `airport`

### Quality
- [x] Golden-file KML test
- [x] OSRM client tests
- [x] Typed-stop and route behavior tests

### Project
- [x] `itineraries/` and `maps/` layout
- [x] MIT license
