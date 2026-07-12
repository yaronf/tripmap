# Roadmap

## Planned

### Routing
- [ ] GraphHopper backend
- [ ] Valhalla backend
- [ ] Offline routing support

### KML
- [ ] Daily colors
- [ ] Driving distance/time
- [ ] Rich HTML descriptions

### Data
- [ ] Shared place database
- [ ] Geocoding
- [ ] Weather annotations
- [ ] Booking links

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

### KML
- [x] Typed stop icons
- [x] Ferry styling
- [x] Hike styling
- [x] Trip, day, and stop descriptions

### Data
- [x] Typed stops: `overnight`, `via`, `attraction`, `viewpoint`, `trailhead`, and `ferry_terminal`

### Quality
- [x] Golden-file KML test
- [x] OSRM client tests
- [x] Typed-stop and route behavior tests