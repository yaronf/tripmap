# Roadmap

## Bugs

- [x] Cannot see stop notes (e.g. flight details on NZ arrive/depart days)
- [x] Comments UI showed text twice while editing (pencil Edit / Done)
- [x] Offline: hide photo UI (and caption/alt) when an image fails to load — don’t show caption alone on mobile

## Planned

### Viewer

See [docs/itinerary-display-viewer.md](docs/itinerary-display-viewer.md) (architecture) and [docs/itinerary-display-ux.md](docs/itinerary-display-ux.md) (UI/UX).

- [x] Phase 1: `--bundle` export (`trip.json`, per-day GeoJSON)
- [x] Phase 2: `viewer/` SPA (day index, map, detail)
- [x] Phase 3: Photo field in YAML + bundle + lightbox
- [x] Phase 4: PWA service worker (offline data + images)
- [x] Phase 5: GitHub Actions → GitHub Pages
- [x] Phase 6: REST API (`cmd/tripmapd/`) + OpenAPI + PATCH (S3 versions)
- [x] Phase 7: Agent MCP (`/mcp` Streamable HTTP + Bearer) via **ChatGPT Agent** — see [docs/runbook-mcp.md](docs/runbook-mcp.md).
- [ ] Phase 8: Cursor skill (optional local alternative)
- [x] Phase 9: Ephemeral PWA comments (`localStorage`)
- [x] Comments should display even when not in edit mode
- [x] Create a favicon
- [x] Mobile: replace prev/next day buttons with swipe left/right
- [x] Map popups: richer HTML (notes / place info), not just the name
- [x] Map markers: replace default points with small typed icons
- [x] Improve display of one-point maps (zoom/framing when a day has only a single marker) — viewer + PDF
- [ ] Agent/MCP: read shared viewer comments (`api/notes`) so itinerary can be updated after the trip

### Schema
- [x] Places registry: define `places:` once, reference by ID in route/stops
- [x] Structured place enrichment (`info`: links, stats, logistics, facilities, warnings, highlights)
- [ ] Stop priority flags: `optional`, `backup`, `must`
- [ ] First-class overnight block (place, nights, notes)
- [ ] Booking metadata on stops (required, opens, status) in KML descriptions
- [ ] Weather backup hints on hike days (`swap_with`) in descriptions
- [x] Trip dates in YAML (`start` + optional per-day `date`) for viewer and KML

### Routing
- [ ] GraphHopper backend
- [ ] Valhalla backend
- [ ] Offline routing support

### KML
- [ ] Daily colors
- [x] Driving distance/time (PWA day list + detail; OSRM when `--route osrm`)
- [ ] Rich HTML descriptions

### Data
- [ ] Geocoding

### AWS / hosting
- [x] Durable public URL: `https://tripmap.sheffer.org` (CloudFront `tripmap-edge` → Express). See [docs/aws-deployment.md](docs/aws-deployment.md).
- [x] Hellō sign-in for human viewers (`/auth/hello/*`, Client ID on compute). Signed-in home lists itineraries at `/me/trips/{id}/`. Allowlist: `config/users.csv` (see `users.example.csv`).
- [x] Remove capability-token sharing (`/t/{id}/{token}/`); Hellō `/me/trips/{id}/` is the viewer path.
- [x] Remove `itineraries/` from the git repo (live SoT is S3; no history rewrite). Local copies stay gitignored under `itineraries/` if needed.

### Export
- [ ] GPX
- [ ] GeoJSON
- [x] PDF archive of itinerary (CLI / Cursor, not served): day sheets + routed map figures for Google Docs etc. (not PWA-pixel-perfect)
- [x] Google Maps links (per-stop search + day directions in viewer)

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
