# Plan: TripIt ICS → tripmap bookings (lodging + flights)

**Status:** draft for implementation  
**Goal:** Keep bookings in TripIt; pull **name, address, and times** (lodging) plus **flight number / route / times** (bracket flights) into tripmap via the existing TripIt **iCal feed**. Show them in the viewer and offer a **Refresh from TripIt** control.

---

## 1. Why ICS (not TripIt API)

- TripIt’s [public API is closed to new integrations](https://help.tripit.com/en/support/solutions/articles/103000391296-tripit-public-api); new OAuth connects are failing in the wild.
- The private [calendar feed](https://help.tripit.com/en/support/solutions/articles/103000063280-calendar-feed-setup-and-sync) still works (already used in personal calendar).
- Read-only, no partner registration, fits a family allowlisted app.

**Non-goals:** live two-way sync, editing bookings in tripmap, TripIt OAuth, full TRIP-style booking UI, auto-moving map pins from ICS geo.

---

## 2. Product shape

| Source of truth | Owns |
|-----------------|------|
| **TripIt** | Reservations, confirmation emails, changes |
| **tripmap YAML** | Days, overnight/airport places, OSRM geometry, viewer experience |
| **ICS sync** | Enrichment only: copy name / address / times onto matched places |

Typical NZ-style trip: **2 air events** (inbound + outbound) bracketing **N lodging** nights.

Viewer must surface, without opening TripIt:

- Lodging: **name, address, check-in, check-out**
- Flight: **name (flight # / route), depart time, arrive time** (terminals/gates if present in ICS)

---

## 3. Schema

Add optional structured `booking` on catalog `places` (not day-local stop overrides). Hydrate into `trip.json` like `info`.

```yaml
places:
  akl-airport:
    title: Auckland Airport
    type: airport
    lat: -37.008
    lon: 174.792
    booking:
      source: tripit
      kind: flight
      name: "UA 917 SFO→AKL"
      # address usually empty for flights
      start: "2026-11-04T21:30:00-07:00"   # depart (ICS DTSTART)
      end: "2026-11-06T05:10:00+13:00"     # arrive (ICS DTEND)
      synced_at: "2026-09-08T19:00:00Z"
      uid: "tripit-vevent-uid-…"            # stable match key

  dannevirke-motel:
    title: Dannevirke Motel
    type: overnight
    lat: …
    lon: …
    booking:
      source: tripit
      kind: lodging
      name: "Dannevirke Motel"
      address: "123 High St, Dannevirke 4930, NZ"
      start: "2026-11-10T15:00:00+13:00"   # check-in
      end: "2026-11-11T10:00:00+13:00"     # check-out
      synced_at: "…"
      uid: "…"
```

**Field rules**

| Field | Lodging | Flight |
|-------|---------|--------|
| `kind` | `lodging` | `flight` |
| `name` | Property name | Flight label (carrier + number + cities if available) |
| `address` | Street / locality | Optional (rare) |
| `start` / `end` | Check-in / check-out | Depart / arrive |
| `uid` | ICS `UID` | ICS `UID` |
| `synced_at` | Last successful sync write | same |

- Do **not** overwrite `title` / `lat` / `lon` / `maps_url` from ICS (pins stay tripmap-owned).
- Prefer RFC3339 with offset; viewer formats in trip-local display.
- Extend `patchTrip` / OpenAPI / MCP so agents can read `booking` and sync can write it. Keep `notes` for free text; booking is structured.

Trip-level metadata (optional, for UI):

```yaml
# on trip root or a sidecar — prefer trip.yaml root
tripit:
  last_sync_at: "…"
  last_sync_ok: true
  last_sync_summary: "2 flights, 12 lodging matched; 0 unmatched"
```

Exact key placement: `Trip.TripIt *TripItMeta` in Go, omitted when unset.

---

## 4. Sync algorithm

### 4.1 Inputs

- `TRIPIT_ICAL_URL` — private feed URL (Secrets Manager + local `.env`; never git).
- Trip YAML: `start`, day count, overnight + airport/flight places.

### 4.2 Parse

Fetch ICS → parse `VEVENT`s. Classify roughly:

| Heuristic | `kind` |
|-----------|--------|
| Summary/categories mention flight / airline / airport codes, or LOCATION looks like airport | `flight` |
| Otherwise lodging-like (hotel, lodging, Airbnb, address-heavy LOCATION) | `lodging` |
| Ambiguous | leave unmatched; include in sync report |

Use a small Go ICS library (or minimal VEVENT parser) — prefer well-tested dependency over hand-rolled folding rules.

### 4.3 Match

**Lodging → overnight places**

1. Build list of overnight endpoints from day routes (night of day *N* = evening lodging on day *N*).
2. Match event date (`DTSTART` date in trip TZ) to that night.
3. If multiple candidates: fuzzy name (`booking.name` / summary vs `place.title`).
4. Persist `uid` after first match so re-sync is stable even if summary text changes slightly.

**Flights → airport / flight places**

1. Prefer places with `type: airport` or `type: flight` on first / last days (and day 1 / last±1).
2. Inbound: earlier `DTEND` near trip `start`; outbound: `DTSTART` near trip end.
3. Fuzzy name / airport code vs place title; then lock `uid`.

**Unmatched events:** listed in API/UI summary; never invent new places automatically in v1 (operator or chat can add a place, then refresh).

### 4.4 Write

- `patchTrip` (or dedicated store helper) updating only `places.<id>.booking` + trip-level `tripit` meta.
- Trigger bundle regen as today’s patches do so `trip.json` updates for viewers.
- Idempotent: same UIDs → rewrite booking fields; no version spam beyond normal patch versioning (one version per refresh is fine).

---

## 5. Backend API

### 5.1 `POST /me/trips/{id}/api/tripit/sync`

- Auth: signed-in session + allowlist (same as `/me/trips/…`).  
  **Who may sync:** any allowlisted user for v1 (family); optional later: `chat=yes` only or an `ops=yes` column.
- Body: empty `{}` (URL from server env, not client).
- Behavior: fetch ICS → match → patch → return JSON:

```json
{
  "ok": true,
  "synced_at": "…",
  "matched": [
    {"uid": "…", "kind": "lodging", "place": "dannevirke-motel", "name": "…"},
    {"uid": "…", "kind": "flight", "place": "akl-airport", "name": "UA 917…"}
  ],
  "unmatched": [
    {"uid": "…", "kind": "lodging", "summary": "…", "start": "…"}
  ],
  "errors": []
}
```

- Errors: missing `TRIPIT_ICAL_URL` → 503; TripIt fetch fail → 502; parse/match soft-fail with `ok: false` + details.
- Rate limit lightly (e.g. once / 30s per trip) so refresh spam doesn’t hammer TripIt.

### 5.2 Agent / MCP / CLI

- Tool: `syncTripIt` (trip-scoped) — same core as HTTP.
- CLI: `tripmap tripit-sync --trip nz-4weeks` for local/Cursor use.
- Chat can call the tool when user says “refresh TripIt” / “pull bookings”.

### 5.3 Infra

| Item | Where |
|------|--------|
| `TRIPIT_ICAL_URL` | Secrets Manager e.g. `tripmap/tripit-ical` → ECS env; `.env` locally |
| Compute | `infra/compute.yaml` + deploy script parameter/secret ref |
| `.env.example` | Placeholder comment only |

Resetting the feed URL in TripIt settings requires updating the secret (document in runbook).

---

## 6. Viewer UI

### 6.1 Display (day detail)

For stops whose catalog place has `booking`:

**Lodging block** (overnight / evening lodging on the day):

- Name  
- Address (if set) — tappable `maps` link if we can build a search URL from address without clobbering pin `maps_url`  
- Check-in / check-out (formatted, with weekday)

**Flight block** (airport / flight on arrive & depart days):

- Flight name / number  
- Depart → arrive times (and date if overnight flight)

Placement: above or beside existing stop notes; quiet styling consistent with `detail-stats` / `stop-notes` (no card stack). Map popup: one-line booking summary when present.

### 6.2 Refresh control

**Trip-level control** in chrome (signed-in `/me/trips/…` only; hide on static Pages / logged-out):

- Button: **Refresh TripIt** (ghost/small next to Days / Ask)  
- During sync: disable + status “Updating bookings…”  
- On success: toast/banner — e.g. `TripIt: 2 flights, 12 stays updated` + optional unmatched count  
- On failure: short error + keep prior `booking` data  
- Show **Last synced** relative time when `tripit.last_sync_at` exists (subtle, under chrome or in a tiny menu)

Optional secondary: same action in Ask chat (“refresh TripIt”) via tool — UI button is the primary path.

**Offline:** button disabled with hint (needs network to TripIt + API).

### 6.3 After sync

Existing trip reload path after mutate (`trip_updated` / refetch `trip.json`) so day detail updates without full page reload when possible; hard refresh acceptable for v1 if simpler.

---

## 7. Security & privacy

- ICS URL is a **capability secret** — anyone with the URL can read the feed. Server-side only; never expose to the browser or OpenAPI examples.
- Sync endpoint must not echo the URL.
- Logs: place ids + kinds + counts; no full address dumps at info level.
- Allowlist still gates who can trigger sync (same as viewing the trip).

---

## 8. Implementation phases

| Phase | Work | Done when |
|-------|------|-----------|
| **A — Schema + parse** | `booking` + `tripit` meta; ICS fetch/parse; unit tests with fixture `.ics` | Golden tests for lodging + flight VEVENTs |
| **B — Match + write** | Matcher; patch path; CLI `tripit-sync` | Dry-run on NZ trip fixture / real feed in staging |
| **C — HTTP API** | `POST …/api/tripit/sync`; secret wiring; deploy | Curl sync as signed-in user updates S3 YAML |
| **D — Viewer UI** | Booking blocks + Refresh button + last-synced | Manual smoke on phone + desktop |
| **E — Agent** | `syncTripIt` tool + chat instruction line | “Refresh TripIt” in Ask works |

Ship A→D for usefulness; E can follow immediately after.

---

## 9. Test plan

- [ ] Fixture ICS: 2 flights + several lodging; assert matches to sample trip YAML  
- [ ] Re-sync stable on `uid` when summary text changes  
- [ ] Unmatched event appears in API `unmatched`, no place invented  
- [ ] Missing secret → 503; button shows clear message  
- [ ] Viewer formats NZ offsets correctly  
- [ ] Refresh updates detail without losing day selection  
- [ ] PDF / KML: booking optional in descriptions later (out of scope for v1 UI); at least do not break builds

---

## 10. Docs / ops checklist

- [ ] Runbook section: enable Calendar Feed in TripIt → copy URL → store secret → deploy  
- [ ] Note: TripIt “Reset calendar feed URL” invalidates secret  
- [ ] `.env.example` + `TODO.md` pointer  
- [ ] Update schema docs / OpenAPI for `booking`

---

## 11. Open decisions (lock at implement start)

1. **Who can refresh?** All allowlisted signed-in users (proposed) vs `chat=yes` only.  
2. **Timezone:** interpret ICS floating times in trip region TZ vs UTC-only — prefer preserve offsets from feed.  
3. **Address → maps link:** derive Google search URL for address text without changing pin coords (proposed yes).  
4. **Auto-create places for unmatched flights/lodging?** No in v1 (proposed).

---

## 12. Success criteria

- On the road, day detail shows lodging name/address/times and bracket flight times without opening TripIt.  
- After a booking change in TripIt, one tap **Refresh TripIt** (or Ask) updates tripmap within a minute.  
- No TripIt API / OAuth dependency.
