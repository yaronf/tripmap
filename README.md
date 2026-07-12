# tripmap (MVP)

Usage:

    go run . --input itinerary.yaml --output trip.kml
    go run . --input itinerary.yaml --output trip.kml --route osrm

tripmap reads a YAML itinerary and emits a KML with one Folder per day,
placemarks for stops, and a route line for each day.

- `--route straight` (default): straight lines between route points.
- `--route osrm`: road routing via the public OSRM server.

## Itinerary schema

```yaml
trip: New Zealand 2026
description: Optional trip description
days:
  - day: 1
    title: Auckland → Waitomo
    notes: Optional note shown on the day's folder.
    route:            # ordered points that shape the day's line
      - { name: Auckland, type: overnight, lat: -36.8485, lon: 174.7633 }
      - { name: Hamilton, type: via,       lat: -37.7870, lon: 175.2793 }
      - { name: Waitomo,  type: overnight, lat: -38.2617, lon: 175.1035 }
    stops:            # map placemarks that are not on the driving line
      - { name: Glowworm Caves, type: attraction, lat: -38.2617, lon: 175.1035 }
```

`route:` and `stops:` are intentionally separate. A day without `route:` has
placemarks only and no route line.

### Stop types

Placement determines behavior: entries in `route:` shape the line, while
entries in `stops:` are placemarks. Route entries also receive placemarks
unless their type is `via`.

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

Styles (`<Style>` icons and line colors) are only emitted for types/flags that
are actually used.
