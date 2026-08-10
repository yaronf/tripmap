# tripmap

tripmap turns a YAML road-trip itinerary into:

- **KML** for Google Earth / My Maps
- **A static PWA** for day-by-day viewing (local or hosted)
- **A hosted service** (CloudFront + `tripmapd`) — Hellō sign-in for human viewers, Bearer-auth **MCP** (and REST) for agents

Each day has map markers and a route line (real roads when OSRM is enabled). The schema separates **where you drive** (`route:`) from **side visits** (`stops:`), with a shared `places:` catalog (schema v2).

## Hosted surfaces

Paths relative to your public base URL (`PUBLIC_BASE_URL`):

| Surface | Path / notes |
|---------|----------------|
| Home / Hellō login | `/` |
| Trip viewer (signed in) | `/me/trips/{id}/` — notes + shared comments |
| Viewer chat (allowlisted) | Persona floating widget → `POST /me/trips/{id}/api/chat` (SSE; needs `OPENAI_API_KEY`) |
| Agent MCP (Codex) | `POST /mcp` — same Bearer as the agent API |
| OpenAPI | `GET /openapi.yaml` (from [`api/openapi.yaml`](api/openapi.yaml); `go generate ./api` regenerates routes) |
| Agent REST | `/api/agent/*` (scripts / smoke; MCP wraps the same handlers) |

- MCP setup: [docs/runbook-mcp.md](docs/runbook-mcp.md) (Codex)
- Seasonal AWS deploy / undeploy: [docs/runbook-deploy-compute.md](docs/runbook-deploy-compute.md), [docs/runbook-undeploy-compute.md](docs/runbook-undeploy-compute.md)
- Architecture: [docs/aws-deployment.md](docs/aws-deployment.md)



## CLI quick start

```bash
# KML for Google Earth (YAML is not in git — fetch via agent API or keep a local copy)
go run . --input trip.yaml --output maps/trip.kml --route osrm

# Local PWA (serve over HTTP — not file://)
go run . --input trip.yaml --bundle maps/trip-bundle/ --route osrm
cd maps/trip-bundle && python3 -m http.server 8080

# PDF archive (cover + overview map + per-day sheets; needs tile + OSRM network)
go run . --trip nz-4weeks --route osrm --pdf maps/nz-4weeks.pdf
# or from local files:
go run . --input trip.yaml --notes notes.json --route osrm --pdf maps/trip.pdf
```

Use exactly one of `--trip` or `--input`. With `--trip`, set `ITINERARIES_BUCKET`, optional `COMMENTS_BUCKET`, and AWS credentials / profile (same as `tripmapd`). Shared viewer comments are included when available; missing notes omit that section.

| Flag                             | Purpose                                                                                      |
| -------------------------------- | -------------------------------------------------------------------------------------------- |
| `--route straight`               | Straight lines (default)                                                                     |
| `--route osrm`                   | Road routing via public [OSRM](https://project-osrm.org/); hike/ferry segments stay straight |
| `--bundle DIR`                   | Write PWA (`trip.json`, `geo/`, embedded viewer)                                             |
| `--pdf PATH`                     | Write PDF archive (overview + day maps, notes, shared comments)                              |
| `--trip ID`                      | Load YAML (+ comments) from S3 by trip id                                                    |
| `--notes PATH`                   | Local shared-comments JSON (with `--input` + `--pdf`)                                        |
| `--mymaps`                       | My Maps–friendly KML (simplify + flatten)                                                    |
| `--simplify M` / `--precision N` | Geometry detail for KML                                                                      |
| `--units km|mi`                  | Distance units in the PWA / PDF                                                              |


`go build -o tripmap .` for a standalone binary. Live itinerary YAML lives in S3 (edit via agent API / MCP); CLI outputs go under `maps/` (gitignored). Viewer source: `internal/bundle/viewer/` (embedded in the CLI and `tripmapd`).

## Itinerary schema (v2)

Places are defined once and referenced by id. Days use `place:` refs (optional day-local `type` / `notes` / `maps_url` overrides).

```yaml
schema_version: 2
trip: Netherlands 2026
description: Two-week road trip
start: "2026-06-22"          # optional; day dates = start + (day − 1)

places:
  amsterdam:
    title: Amsterdam
    lat: 52.3676
    lon: 4.9041
    type: overnight
  pancake-rocks:
    title: Pancake Rocks
    lat: -42.1148
    lon: 171.3260
    type: attraction
    maps_url: https://maps.app.goo.gl/…   # optional; pin opens this URL
    # info: { links, stats, warnings, … }  # optional enrichment

days:
  - day: 9
    title: Nelson → Punakaiki
    notes: Human day narrative (agents should not edit unless asked).
    route:                               # shapes the driving/hike line
      - { place: nelson, type: overnight }
      - { place: murchison, type: via }
      - { place: punakaiki, type: overnight }
    stops:                               # markers only; do not suppress the route
      - { place: pancake-rocks, type: attraction }
```

- `route:` — polyline (needs ≥2 points). `via` shapes the line without a marker.
- `stops:` — placemarks only; independent of whether a route is drawn.
- **Photos** — `photo` / `photo_caption` on days or places (HTTPS URL or path relative to the YAML).



### Stop types


| Type                                     | Marker                | Typical use                     |
| ---------------------------------------- | --------------------- | ------------------------------- |
| *(none)*                                 | default               | generic point                   |
| `overnight`                              | lodging               | lodging / day endpoint          |
| `depart`                                 | (viewer)              | morning lodging on a travel day |
| `hut`                                    | hut                   | backcountry hut                 |
| `via`                                    | none                  | hidden route waypoint           |
| `attraction` / `viewpoint` / `trailhead` | star / camera / hiker | side visits                     |
| `ferry_terminal` / `airport` / `flight`  | ferry / airport       | terminals & flights             |




### Day flags

- `hike: true` — trail segments straight; approaches may use OSRM
- `ferry: true` — ferry-terminal pairs straight (orange); other segments may use OSRM



## Viewer

Desktop: day list | map | detail. Phone: List / Map + day picker. Pins open Google Maps (`maps_url` or lat/lon). Shared comments (signed-in) live on the host; the service worker keeps `trip.json` and `geo/` network-first so itinerary edits show up without a stale map.

UI notes: [docs/itinerary-display-ux.md](docs/itinerary-display-ux.md).

## Google Earth / My Maps

Google Earth Pro is the best KML viewer. Expand each day folder and enable the **Route** placemark if the line is missing.

| Style | Color | Use |
|-------|-------|-----|
| `driveLine` | blue | driving |
| `hikeLine` | green | `hike: true` |
| `ferryLine` | orange | `ferry: true` |

For My Maps, generate with `--mymaps` (flatten + simplify).

## Hosted daemon (`tripmapd`)

`cmd/tripmapd` serves viewers, agent API, MCP ([mcpopenapi](https://github.com/yaronf/mcpopenapi)), and bundle regeneration against S3. In season it runs on ECS Express Mode behind CloudFront; off season delete compute to stop ALB/Fargate charges (data stays in S3).

## Tests

```bash
go test ./...
```



## Roadmap

See [TODO.md](TODO.md).

## License

MIT — see [LICENSE](LICENSE).