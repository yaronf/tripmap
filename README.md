# tripmap

tripmap turns a YAML road-trip itinerary into a KML file you can open in Google
Earth. Each day becomes a folder with map markers for stops and a route line
showing how you get from A to B — following real roads when OSRM routing is
enabled.

The project is aimed at multi-day driving trips where some days are on the road,
some are hikes, and some involve ferries or side trips. The YAML schema separates
**where you drive** (`route:`) from **what you visit** (`stops:`), so a town you
pass through to shape the route does not have to clutter the map.

## Quick start

```bash
go run . --input itinerary.yaml --output trip.kml
go run . --input long-itinerary.yaml --output long-trip.kml --route osrm
```

- `--route straight` (default): straight lines between route points.
- `--route osrm`: road routing via the public [OSRM](https://project-osrm.org/)
  demo server. Hike and ferry days stay straight regardless of this flag.

Build a standalone binary with `go build -o tripmap .`.

## Viewing the output

**Google Earth Pro** (desktop) is the best viewer. On Windows with WSL, open the
file from the Windows side:

```
\\wsl$\<distro>\home\<user>\<path-to-project>\long-trip.kml
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
reliably; use Google Earth if routes are missing.

## Itinerary schema

```yaml
trip: New Zealand 2026
description: Four-week road trip starting 22 Nov 2026
days:
  - day: 9
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
placemarks only and no route line.

See `long-itinerary.yaml` for a full 28-day example with driving days, hikes,
a ferry crossing, and side-trip attractions.

### Stop types

Placement determines behavior: entries in `route:` shape the line, while entries
in `stops:` are placemarks. Route entries also receive placemarks unless their
type is `via`.

| Type             | Marker  | Typical use |
|------------------|---------|-------------|
| _(none)_         | default | generic point |
| `overnight`      | lodging | route endpoint or lodging placemark |
| `via`            | none    | hidden route-shaping waypoint |
| `attraction`     | star    | attraction placemark |
| `viewpoint`      | camera  | viewpoint placemark |
| `trailhead`      | hiker   | hike endpoint |
| `ferry_terminal` | ferry   | ferry endpoint |

### Day flags

- `hike: true` — draw the day's line straight (no road routing), styled green.
- `ferry: true` — draw the day's line straight, styled as a ferry crossing.

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
