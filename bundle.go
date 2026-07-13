package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
)

// TripJSON is the viewer-facing trip metadata (no heavy geometry).
type TripJSON struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description,omitempty"`
	Units       string    `json:"units"` // km | mi
	Days        []DayJSON `json:"days"`
}

type DayJSON struct {
	Day          int        `json:"day"`
	Title        string     `json:"title"`
	Notes        string     `json:"notes,omitempty"`
	Hike         bool       `json:"hike,omitempty"`
	Ferry        bool       `json:"ferry,omitempty"`
	Photo        string     `json:"photo,omitempty"`
	PhotoCaption string     `json:"photo_caption,omitempty"`
	Stops        []StopJSON `json:"stops"`
	Geo          string     `json:"geo"`
	Kind         string     `json:"kind"`                 // drive | hike | ferry | rest
	DriveDist    float64    `json:"drive_dist,omitempty"` // in trip units (km or mi)
	DriveMin     int        `json:"drive_min,omitempty"`  // OSRM estimate; omitted for straight
}

type StopJSON struct {
	Name         string  `json:"name"`
	Type         string  `json:"type,omitempty"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	Notes        string  `json:"notes,omitempty"`
	Photo        string  `json:"photo,omitempty"`
	PhotoCaption string  `json:"photo_caption,omitempty"`
}

type geoFeatureCollection struct {
	Type     string       `json:"type"`
	Features []geoFeature `json:"features"`
}

type geoFeature struct {
	Type       string         `json:"type"`
	Properties map[string]any `json:"properties"`
	Geometry   geoGeometry    `json:"geometry"`
}

type geoGeometry struct {
	Type        string `json:"type"`
	Coordinates any    `json:"coordinates"`
}

func tripIDFromPath(inputPath string) string {
	base := filepath.Base(inputPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func dayKind(d Day) string {
	switch {
	case d.Ferry:
		return "ferry"
	case d.Hike:
		return "hike"
	case len(effectiveRoutePoints(d)) >= 2:
		return "drive"
	default:
		return "rest"
	}
}

func buildTripBundle(ctx context.Context, t Trip, inputPath, outDir string, opts RouteOptions) error {
	id := tripIDFromPath(inputPath)
	if err := os.MkdirAll(filepath.Join(outDir, "geo"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(outDir, "images"), 0755); err != nil {
		return err
	}

	inputDir := filepath.Dir(inputPath)
	photoMap := map[string]string{} // src path -> images/ relative for de-dupe naming
	units := opts.Units
	if units == "" {
		units = "km"
	}
	tj := TripJSON{ID: id, Title: t.Trip, Description: t.Description, Units: units}

	for _, d := range t.Days {
		dj, err := buildDayBundle(ctx, d, inputDir, outDir, opts, photoMap)
		if err != nil {
			return fmt.Errorf("day %d: %w", d.Day, err)
		}
		tj.Days = append(tj.Days, dj)
	}

	tripBytes, err := json.MarshalIndent(tj, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outDir, "trip.json"), append(tripBytes, '\n'), 0644); err != nil {
		return err
	}

	if err := copyViewerAssets(outDir); err != nil {
		return fmt.Errorf("copy viewer: %w", err)
	}

	manifest := fmt.Sprintf(`{
  "name": %q,
  "short_name": %q,
  "start_url": "./index.html",
  "display": "standalone",
  "background_color": "#f3efe6",
  "theme_color": "#0f5c5c",
  "icons": [{"src": "icon.svg", "sizes": "any", "type": "image/svg+xml", "purpose": "any"}]
}
`, t.Trip, t.Trip)
	if err := os.WriteFile(filepath.Join(outDir, "manifest.webmanifest"), []byte(manifest), 0644); err != nil {
		return err
	}

	return writeServiceWorker(outDir, tj)
}

func buildDayBundle(ctx context.Context, d Day, inputDir, outDir string, opts RouteOptions, photoMap map[string]string) (DayJSON, error) {
	geoName := fmt.Sprintf("geo/day-%02d.json", d.Day)
	dj := DayJSON{
		Day:          d.Day,
		Title:        d.Title,
		Notes:        d.Notes,
		Hike:         d.Hike,
		Ferry:        d.Ferry,
		PhotoCaption: d.PhotoCaption,
		Kind:         dayKind(d),
		Geo:          geoName,
		Stops:        []StopJSON{},
	}

	if d.Photo != "" {
		rel, err := copyPhoto(d.Photo, inputDir, outDir, photoMap)
		if err != nil {
			return DayJSON{}, err
		}
		dj.Photo = rel
	}

	seen := map[string]bool{}
	addStop := func(s Stop) error {
		if s.Type == "via" {
			return nil
		}
		key := s.Name + "|" + s.Type + "|" + stopKey(s)
		if seen[key] {
			return nil
		}
		seen[key] = true
		sj := StopJSON{Name: s.Name, Type: s.Type, Lat: s.Lat, Lon: s.Lon, Notes: s.Notes, PhotoCaption: s.PhotoCaption}
		if s.Photo != "" {
			rel, err := copyPhoto(s.Photo, inputDir, outDir, photoMap)
			if err != nil {
				return err
			}
			sj.Photo = rel
		}
		dj.Stops = append(dj.Stops, sj)
		return nil
	}
	for _, s := range viewerDayStops(d) {
		if err := addStop(s); err != nil {
			return DayJSON{}, err
		}
	}

	fc := geoFeatureCollection{Type: "FeatureCollection"}
	for _, s := range dj.Stops {
		fc.Features = append(fc.Features, geoFeature{
			Type: "Feature",
			Properties: map[string]any{
				"name": s.Name,
				"type": s.Type,
				"kind": "stop",
			},
			Geometry: geoGeometry{Type: "Point", Coordinates: []float64{s.Lon, s.Lat}},
		})
	}

	rp := effectiveRoutePoints(d)
	if len(rp) >= 2 {
		segs, err := buildRouteSegments(ctx, d, rp, opts)
		if err != nil {
			return DayJSON{}, err
		}
		var driveM, driveS float64
		for _, seg := range segs {
			props := map[string]any{
				"name":  seg.Name,
				"style": seg.Style,
				"kind":  "route",
			}
			if seg.DistanceMeters > 0 {
				props["distance_m"] = math.Round(seg.DistanceMeters)
			}
			if seg.DurationSeconds > 0 {
				props["duration_s"] = math.Round(seg.DurationSeconds)
			}
			fc.Features = append(fc.Features, geoFeature{
				Type:       "Feature",
				Properties: props,
				Geometry:   geoGeometry{Type: "LineString", Coordinates: seg.Geometry},
			})
			if seg.Style == "driveLine" {
				driveM += seg.DistanceMeters
				driveS += seg.DurationSeconds
			}
		}
		if driveM > 0 {
			dj.DriveDist = distanceInUnits(driveM, opts.Units)
		}
		if driveS > 0 {
			dj.DriveMin = int(math.Round(driveS / 60))
		}
	}

	b, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return DayJSON{}, err
	}
	if err := os.WriteFile(filepath.Join(outDir, geoName), append(b, '\n'), 0644); err != nil {
		return DayJSON{}, err
	}
	return dj, nil
}

func distanceInUnits(meters float64, units string) float64 {
	var dist float64
	switch units {
	case "mi":
		dist = meters / 1609.344
	default:
		dist = meters / 1000
	}
	return math.Round(dist*10) / 10 // one decimal
}

func copyPhoto(src, inputDir, outDir string, photoMap map[string]string) (string, error) {
	if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") {
		return src, nil
	}
	abs := src
	if !filepath.IsAbs(src) {
		abs = filepath.Join(inputDir, src)
	}
	if rel, ok := photoMap[abs]; ok {
		return rel, nil
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return "", fmt.Errorf("photo %s: %w", src, err)
	}
	base := filepath.Base(src)
	destRel := filepath.ToSlash(filepath.Join("images", base))
	destAbs := filepath.Join(outDir, "images", base)
	// If another source already claimed this basename in this build, uniquify.
	if other, ok := photoMap[destAbs]; ok && other != abs {
		ext := filepath.Ext(base)
		stem := strings.TrimSuffix(base, ext)
		for i := 2; ; i++ {
			cand := fmt.Sprintf("%s-%d%s", stem, i, ext)
			destRel = filepath.ToSlash(filepath.Join("images", cand))
			destAbs = filepath.Join(outDir, "images", cand)
			if _, taken := photoMap[destAbs]; !taken {
				if _, err := os.Stat(destAbs); os.IsNotExist(err) {
					break
				}
			}
		}
	}
	if err := os.WriteFile(destAbs, data, 0644); err != nil {
		return "", err
	}
	photoMap[abs] = destRel
	photoMap[destAbs] = abs // mark basename ownership for collision checks
	return destRel, nil
}

func copyViewerAssets(outDir string) error {
	return fs.WalkDir(viewerFS, "viewer", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel("viewer", path)
		if err != nil {
			return err
		}
		if rel == "manifest.webmanifest" {
			return nil // written per-trip
		}
		dest := filepath.Join(outDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		src, err := viewerFS.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.Create(dest)
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, src)
		return err
	})
}

func writeServiceWorker(outDir string, tj TripJSON) error {
	assets := []string{
		"./",
		"./index.html",
		"./app.js",
		"./style.css",
		"./manifest.webmanifest",
		"./icon.svg",
		"./trip.json",
	}
	for _, d := range tj.Days {
		assets = append(assets, "./"+d.Geo)
		if d.Photo != "" && !strings.HasPrefix(d.Photo, "http") {
			assets = append(assets, "./"+d.Photo)
		}
		for _, s := range d.Stops {
			if s.Photo != "" && !strings.HasPrefix(s.Photo, "http") {
				assets = append(assets, "./"+s.Photo)
			}
		}
	}
	list, err := json.Marshal(assets)
	if err != nil {
		return err
	}
	sw := fmt.Sprintf(`/* generated by tripmap */
const CACHE = %q;
const ASSETS = %s;

self.addEventListener("install", (e) => {
  e.waitUntil(caches.open(CACHE).then((c) => c.addAll(ASSETS)).then(() => self.skipWaiting()));
});

self.addEventListener("activate", (e) => {
  e.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener("fetch", (e) => {
  const url = new URL(e.request.url);
  if (url.origin !== self.location.origin) {
    const isImage =
      e.request.destination === "image" ||
      /\.(jpe?g|png|gif|webp|svg)(\?|$)/i.test(url.pathname);
    e.respondWith(
      (isImage ? caches.match(e.request) : Promise.resolve(undefined)).then((cached) => {
        if (cached) return cached;
        return fetch(e.request).then((res) => {
          // Opaque responses (typical for <img> hotlinks) are cacheable; status is 0.
          if (isImage && res && (res.ok || res.type === "opaque")) {
            const copy = res.clone();
            caches.open(CACHE).then((c) => c.put(e.request, copy)).catch(() => {});
          }
          return res;
        });
      }).catch(() => (isImage ? caches.match(e.request) : Promise.reject()))
    );
    return;
  }
  e.respondWith(
    caches.match(e.request).then((cached) => cached || fetch(e.request).then((res) => {
      const copy = res.clone();
      caches.open(CACHE).then((c) => c.put(e.request, copy));
      return res;
    }))
  );
});
`, "tripmap-"+tj.ID+"-v19", string(list))
	return os.WriteFile(filepath.Join(outDir, "sw.js"), []byte(sw), 0644)
}
