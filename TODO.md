# Roadmap

## Planned

### Viewer

See [docs/itinerary-display-viewer.md](docs/itinerary-display-viewer.md) (architecture) and [docs/itinerary-display-ux.md](docs/itinerary-display-ux.md) (UI/UX).

- [ ] Phase 1: `--bundle` export (`trip.json`, per-day GeoJSON)
- [ ] Phase 2: `viewer/` SPA (day index, map, detail)
- [ ] Phase 3: Photo field in YAML + bundle + lightbox
- [ ] Phase 4: PWA service worker (offline data + images)
- [ ] Phase 5: GitHub Actions → GitHub Pages
- [ ] Phase 6: REST API (`cmd/tripmapd/`) + OpenAPI + PATCH → git
- [ ] Phase 7: Custom GPT Action
- [ ] Phase 8: Cursor skill (optional local alternative)
- [ ] Phase 9: Ephemeral PWA comments (`localStorage`)

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
