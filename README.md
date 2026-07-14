# tripmap

tripmap turns a YAML road-trip itinerary into a KML file for Google Earth / My
Maps, or a static PWA for day-by-day viewing on phone and laptop. Each day has
map markers and a route line — following real roads when OSRM routing is enabled.

The project is aimed at multi-day driving trips where some days are on the road,
some are hikes, and some involve ferries or side trips. The YAML schema separates
**where you drive** (`route:`) from **what you visit** (`stops:`), so a town you
pass through to shape the route does not have to clutter the map.

## Quick start

```bash
# KML for Google Earth
go run . --input itineraries/holland.yaml --output maps/holland.kml --route osrm

# PWA bundle (serve over HTTP — do not open file://)
go run . --input itineraries/holland.yaml --bundle maps/holland-bundle/ --route osrm --mymaps
cd maps/holland-bundle && python3 -m http.server 8080
# visit http://localhost:8080/

go run . --input itineraries/nz-4weeks.yaml --output maps/nz-4weeks.kml --route osrm --mymaps
go run .
```

- `--route straight` (default): straight lines between route points.
- `--route osrm`: road routing via the public [OSRM](https://project-osrm.org/)
  demo server. Hike and ferry days stay straight regardless of this flag.
- `--bundle DIR`: write a static PWA (`trip.json`, `geo/`, embedded viewer).
  With `--route osrm`, simplification defaults to 100 m unless overridden.
- `--mymaps`: optimize for [Google My Maps](https://support.google.com/mymaps/answer/3024836)
  import — 100 m Douglas-Peucker simplification, 5-decimal coordinates, and a
  flat placemark layout (My Maps ignores KML Folders). Equivalent to
  `-simplify 100 -precision 5` plus flattening.
- `--simplify METERS`: Douglas-Peucker simplify of full OSRM geometry (meters).
  `0` keeps full detail (best for Google Earth).
- `--precision N`: decimal places for coordinates in the KML (default 6).
- `--units km|mi`: distance units in the PWA bundle (default `km`). No in-viewer toggle yet.

Build a standalone binary with `go build -o tripmap .`.

Itinerary YAML files live in `itineraries/`; generated KML and PWA bundles go in
`maps/` (gitignored). Viewer source is in `viewer/` (embedded into the CLI).
Test fixtures remain in `testdata/`.

## Viewing the output

### PWA (day-by-day navigation)

```bash
go run . --input itineraries/holland.yaml --bundle maps/holland-bundle/ --route osrm --mymaps
cd maps/holland-bundle && python3 -m http.server 8080
```

Desktop: day list | map | detail. Phone: List / Map toggle and a day picker.
Optional local notes are stored in `localStorage` only. The service worker caches
trip data and images after the first visit; basemap tiles still need the network.

Optional photos on days or stops — local path (relative to the YAML) or HTTPS URL:

```yaml
photo: https://upload.wikimedia.org/wikipedia/commons/thumb/…/1280px-….jpg
photo_caption: Amsterdam canals at blue hour   # hover + lightbox text
# or: photo: photos/harbour.jpg
```

Local files are copied into the bundle; URLs are kept as-is (network required on first view; images cache after load).

See [docs/itinerary-display-ux.md](docs/itinerary-display-ux.md) for UI design and
[docs/itinerary-display-viewer.md](docs/itinerary-display-viewer.md) for the longer roadmap.

### Public site (GitHub Pages)

Pushing to `main` (or running **Deploy Pages** manually) builds every
`itineraries/*.yaml` and publishes:

| Trip | URL |
|------|-----|
| Index | https://www.sheffer.org/tripmap/ |
| Holland | https://www.sheffer.org/tripmap/trips/holland/ |
| NZ | https://www.sheffer.org/tripmap/trips/nz-4weeks/ |

(Also reachable as `https://yaronf.github.io/tripmap/…`.)

**One-time repo setup:** Settings → Pages → **Source: GitHub Actions** (already
enabled for this repo).

**Custom domain notes:** this site is served under `/tripmap/` on
`www.sheffer.org`. If you later put a one-line hostname in a repo-root `CNAME`
file, the workflow copies it into the published site — only do that if that
hostname should map cleanly to this Pages deployment (and will not fight another
app on the same host).

### Google Earth / My Maps

**Google Earth Pro** (desktop) is the best KML viewer. On Windows with WSL, open the
file from the Windows side:

```
\\wsl$\<distro>\home\<user>\<path-to-project>\maps\nz-4weeks.kml
```

In the Places sidebar, expand each day folder and make sure the **Route**
placemark is checked. Route lines are color-coded:

| Style | Color | Used for |
|-------|-------|----------|
| `driveLine` | blue | driving days (straight or OSRM) |
| `hikeLine` | green | days with `hike: true` |
| `ferryLine` | orange | days with `ferry: true` |

Days with only a single overnight stop and no `route:` list show markers but no
line — rest days, explore days, etc.

Google My Maps and other lightweight KML viewers may not render route lines
reliably; use Google Earth if routes are missing. For My Maps, generate with
`--mymaps` — My Maps does not support KML Folders and splits long lines at
500 points, so the output is flattened and simplified accordingly.

## Itinerary schema

```yaml
trip: New Zealand 2026
description: Four-week road trip starting 22 Nov 2026
start: 2026-11-22   # optional; fills day dates as start + (day − 1)
days:
  - day: 9
    # date: 2026-11-30   # optional override (YYYY-MM-DD)
    title: Nelson → Punakaiki
    notes: Murchison and Westport are drive-through towns.
    route:            # ordered points that shape the day's line
      - { name: Nelson,    type: overnight, lat: -41.2706, lon: 173.2840 }
      - { name: Murchison, type: via,       lat: -41.8092, lon: 172.3330 }
      - { name: Westport,  type: via,       lat: -41.7526, lon: 171.6034 }
      - { name: Punakaiki, type: overnight, lat: -42.1117, lon: 171.3245 }
    stops:            # map placemarks not on the driving line
      - { name: Pancake Rocks, type: attraction, lat: -42.1148, lon: 171.3260 }
```

`route:` and `stops:` are intentionally separate. A day without `route:` has
placemarks only and no route line. Calendar dates are optional: omit `start`
and `date` for day-number-only trips; when present, the PWA day index and
detail view show them.

See `itineraries/nz-4weeks.yaml` for a full 28-day example with driving days, hikes,
a ferry crossing, and side-trip attractions.

### Stop types

Placement determines behavior: entries in `route:` shape the line, while entries
in `stops:` are placemarks. Route entries also receive placemarks unless their
type is `via`.

| Type             | Marker  | Typical use |
|------------------|---------|-------------|
| _(none)_         | default | generic point |
| `overnight`      | lodging | route endpoint or lodging placemark |
| `hut`            | hut     | backcountry hut on a multi-day hike |
| `via`            | none    | hidden route-shaping waypoint |
| `attraction`     | star    | attraction placemark |
| `viewpoint`      | camera  | viewpoint placemark |
| `trailhead`      | hiker   | hike endpoint |
| `ferry_terminal` | ferry   | ferry endpoint |
| `airport`        | airport | airport (arrival, departure, car pickup) |

Optional fields: `notes`, `photo` (day or stop) — `photo` may be an HTTPS URL
or a path relative to the itinerary file (copied into PWA bundles).

### Day flags

- `hike: true` — trail segments are straight lines; driving approaches from
  towns to trailheads use OSRM when `--route osrm` is set.
- `ferry: true` — ferry-terminal pairs are drawn straight (orange); other
  segments on the day use OSRM when `--route osrm` is set.

Each location appears once on the map even if it is visited on multiple days.
On hike days, lodging listed under `stops:` is automatically prepended to the
route when the trail does not already start there.

Styles (`<Style>` icons and line colors) are only emitted for types and flags
that actually appear in the itinerary.

## Tests

```bash
go test ./...
```

The test suite includes a golden-file check for KML output, OSRM client tests
with a mock server, and tests for typed-stop and route behavior.

## Roadmap

See [TODO.md](TODO.md) for planned features (GraphHopper/Valhalla backends,
driving distance/time in KML, GPX export, itinerary validation, etc.).

## License

MIT — see [LICENSE](LICENSE).
