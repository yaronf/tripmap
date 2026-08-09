package httpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yaronf/mcpopenapi"
	"github.com/yaronf/tripmap/api"
	"github.com/yaronf/tripmap/internal/bundle"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/routebuild"
	"github.com/yaronf/tripmap/internal/store"
)

var _ api.ServerInterface = (*Server)(nil)

// Server is the tripmapd HTTP API.
type Server struct {
	cfg   Config
	store store.Store
	mux   *http.ServeMux
}

// New builds the HTTP server.
func New(cfg Config, st store.Store) *Server {
	s := &Server{cfg: cfg, store: st, mux: http.NewServeMux()}
	s.routes()
	return s
}

// Handler returns the root handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPI)
	s.mux.HandleFunc("GET /favicon.svg", s.handleFavicon)
	s.mux.HandleFunc("GET /favicon.ico", s.handleFavicon)
	s.mux.HandleFunc("GET /{$}", s.handleRoot)
	s.mux.HandleFunc("GET /auth/hello/login", s.handleHelloLogin)
	s.mux.HandleFunc("GET /auth/hello/callback", s.handleHelloCallback)
	s.mux.HandleFunc("GET /auth/me", s.handleAuthMe)
	s.mux.HandleFunc("GET /auth/logout", s.handleAuthLogout)
	s.mux.Handle("/me/trips/", http.HandlerFunc(s.handleSessionTrip))

	// Spec-driven routes (same mux for REST under bearer and MCP upstream without bearer).
	specMux := http.NewServeMux()
	api.HandlerFromMux(s, specMux)
	s.mux.Handle("GET /health", http.HandlerFunc(specMux.ServeHTTP))
	s.mux.Handle("/api/agent/", bearerAuth(s.cfg.AgentBearerToken, http.HandlerFunc(specMux.ServeHTTP)))

	mcpHandler, err := mcpopenapi.NewHandler(mcpopenapi.Config{
		Name:    "tripmap",
		Version: "0.4.0",
		Instructions: "tripmap agent API as MCP tools. Prefer patchTrip with update_day or places.<id>.info; " +
			"do not put enrichment in notes unless the user asks. listTrips then getTrip/getSchema before edits. " +
			"Human viewers sign in with Hellō, then use /me/trips/{id}/.",
		// Concrete servers URL (placeholder {{BASE_URL}} is not valid YAML for the parser).
		OpenAPIYAML: []byte(OpenAPIDocument("https://tripmap.local")),
		Upstream:    specMux,
		PathPrefix:  "/api/agent",
	})
	if err != nil {
		// Fail closed at startup — misconfigured OpenAPI must not ship a half-broken daemon.
		panic("mcp: " + err.Error())
	}
	s.mux.Handle("/mcp", bearerAuth(s.cfg.AgentBearerToken, mcpHandler))
	s.mux.Handle("/mcp/", bearerAuth(s.cfg.AgentBearerToken, mcpHandler))
}

type mutateResult struct {
	ID            string `json:"id"`
	VersionID     string `json:"version_id,omitempty"`
	SchemaVersion int    `json:"schema_version"`
	ViewerURL     string `json:"viewer_url,omitempty"`
	BundleOK      bool   `json:"bundle_ok"`
	BundleError   string `json:"bundle_error,omitempty"`
}

func (s *Server) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	hello := s.helloEnabled()
	sess, authed := s.sessionFromRequest(r)
	var body string
	if !hello {
		body = `<p>tripmapd — Hellō login not configured (<code>HELLO_CLIENT_ID</code>).</p>`
	} else if authed {
		body = fmt.Sprintf(`
<p>Signed in as <strong>%s</strong> (%s)</p>
<p><a href="/auth/logout">Sign out</a></p>
%s
<p class="muted">Open an itinerary above to view the map and leave comments.</p>`,
			htmlEscape(sess.Name), htmlEscape(sess.Email), s.tripListHTML(r))
	} else {
		body = `
<link href="https://cdn.hello.coop/css/hello-btn.css" rel="stylesheet"/>
<p>tripmap seasonal API &amp; viewers.</p>
<div class="hello-container">
  <button class="hello-btn" type="button" onclick="login(event)">ō&nbsp;&nbsp;Continue with Hellō</button>
</div>
<script>
function login(event){
  event.target.classList.add('hello-btn-loader');
  event.target.disabled = true;
  window.location.href = '/auth/hello/login';
}
</script>
<p class="muted">Sign in to browse itineraries.</p>`
	}
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"/><meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>tripmap</title>
<link rel="icon" href="/favicon.svg" type="image/svg+xml"/>
<style>body{font-family:system-ui,sans-serif;max-width:36rem;margin:2.5rem auto;padding:0 1rem;line-height:1.5;color:#1a1f1c;background:#f3efe6}
a{color:#0f5c5c}code{font-size:.9em}.muted{color:#5c6560;font-size:.9rem}ul.trips{list-style:none;padding:0;margin:1.25rem 0}ul.trips li{margin:.55rem 0;padding:.55rem 0;border-top:1px solid #ddd6c8}ul.trips li:first-child{border-top:0}ul.trips a{font-weight:600;text-decoration:none}ul.trips a:hover{text-decoration:underline}ul.trips .id{display:block;font-size:.85rem;color:#5c6560;font-weight:400}</style>
</head><body><h1>tripmap</h1>%s</body></html>`, body)
}

func (s *Server) tripListHTML(r *http.Request) string {
	ids, err := s.store.ListTripIDs(r.Context())
	if err != nil {
		return `<p>Could not load itineraries.</p>`
	}
	if len(ids) == 0 {
		return `<h2>Itineraries</h2><p class="muted">No itineraries yet.</p>`
	}
	var b strings.Builder
	b.WriteString(`<h2>Itineraries</h2><ul class="trips">`)
	for _, id := range ids {
		title := id
		if obj, err := s.store.GetYAML(r.Context(), id); err == nil {
			if trip, err := itinerary.ParseYAML(obj.Body); err == nil {
				if t := strings.TrimSpace(trip.Trip); t != "" {
					title = t
				}
			}
		}
		fmt.Fprintf(&b, `<li><a href="/me/trips/%s/">%s</a><span class="id">%s</span></li>`,
			htmlEscape(id), htmlEscape(title), htmlEscape(id))
	}
	b.WriteString(`</ul>`)
	return b.String()
}

func htmlEscape(s string) string {
	repl := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return repl.Replace(s)
}

func (s *Server) ListTrips(w http.ResponseWriter, r *http.Request) {
	ids, err := s.store.ListTripIDs(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if ids == nil {
		ids = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"trips": ids})
}

func (s *Server) GetSchema(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": itinerary.CurrentSchemaVersion,
		"description":    "tripmap itinerary YAML schema with places catalog",
		"fields": map[string]any{
			"schema_version": "int (required on write; server injects 2 if omitted)",
			"trip":           "string title",
			"description":    "optional string",
			"start":          "optional YYYY-MM-DD",
			"places":         "map of place id -> {title, lat, lon, type?, notes?, photo?, photo_caption?, maps_url?, info?}",
			"days":           "array of day objects; route/stops are {place, type?, notes?, maps_url?}",
		},
		"patch_ops": []string{"swap_days", "update_day", "days", "places", "upsert_stop", "remove_stop", "insert_day", "delete_day"},
		"notes_policy": "Day and stop notes are human-authored. Agents should not modify them unless the user explicitly asks. Put enrichment in places.*.info. Not enforced by the API.",
		"update_day_example": map[string]any{
			"update_day": map[string]any{
				"day":   1,
				"title": "Arrive Auckland",
				"notes": "Recover from flight after arriving on UA917 from SFO at 09:10 NZDT.",
			},
		},
		"days_patch_example": map[string]any{
			"days": map[string]any{
				"8": map[string]string{"title": "New title for day 8"},
			},
		},
		"places_patch_example": map[string]any{
			"places": map[string]any{
				"tongariro-crossing": map[string]any{
					"info": map[string]any{
						"links": []map[string]string{
							{"type": "alltrails", "title": "AllTrails", "url": "https://example.com"},
						},
					},
				},
			},
		},
	})
}

func (s *Server) GetTrip(w http.ResponseWriter, r *http.Request, id string) {
	if !s.requireTripID(w, id) {
		return
	}
	obj, err := s.store.GetYAML(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	trip, err := itinerary.ParseYAML(obj.Body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta, _ := s.store.GetMeta(r.Context(), id)
	writeJSON(w, http.StatusOK, map[string]any{
		"id":             id,
		"version_id":     obj.VersionID,
		"schema_version": trip.SchemaVersion,
		"trip":           trip.Trip,
		"description":    trip.Description,
		"start":          trip.Start,
		"days":           len(trip.Days),
		"updated_at":     meta.UpdatedAt,
	})
}

func (s *Server) GetTripYAML(w http.ResponseWriter, r *http.Request, id string) {
	if !s.requireTripID(w, id) {
		return
	}
	obj, err := s.store.GetYAML(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("X-Tripmap-Version-Id", obj.VersionID)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(obj.Body)
}

func (s *Server) CreateTrip(w http.ResponseWriter, r *http.Request, _ api.CreateTripParams) {
	if err := s.requireIdempotency(w, r); err != nil {
		return
	}
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	id, yamlBytes, err := parseCreateBody(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := itinerary.ValidateID(id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	exists, err := s.store.Exists(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if exists {
		writeErr(w, http.StatusConflict, fmt.Errorf("trip %q already exists", id))
		return
	}

	trip, err := prepareTripYAML(yamlBytes)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	now := time.Now().UTC()
	meta := store.Meta{SchemaVersion: trip.SchemaVersion, CreatedAt: now, UpdatedAt: now}

	res, status, err := s.commitMutate(r.Context(), id, outYAML, &meta)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	s.finishIdempotent(w, r, http.StatusCreated, res)
}

func (s *Server) PutTripYAML(w http.ResponseWriter, r *http.Request, id string, _ api.PutTripYAMLParams) {
	if err := s.requireIdempotency(w, r); err != nil {
		return
	}
	if err := itinerary.ValidateID(id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	exists, err := s.store.Exists(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if !exists {
		writeErr(w, http.StatusNotFound, fmt.Errorf("trip %q not found", id))
		return
	}
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	trip, err := prepareTripYAML(body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta, err := s.store.GetMeta(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta.SchemaVersion = trip.SchemaVersion
	meta.UpdatedAt = time.Now().UTC()
	res, status, err := s.commitMutate(r.Context(), id, outYAML, &meta)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	s.finishIdempotent(w, r, http.StatusOK, res)
}

func (s *Server) PatchTrip(w http.ResponseWriter, r *http.Request, id string, _ api.PatchTripParams) {
	if err := s.requireIdempotency(w, r); err != nil {
		return
	}
	if !s.requireTripID(w, id) {
		return
	}
	obj, err := s.store.GetYAML(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	trip, err := itinerary.ParseYAML(obj.Body)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var p itinerary.Patch
	if err := json.Unmarshal(body, &p); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("invalid patch json: %w", err))
		return
	}
	if err := itinerary.ApplyPatch(&trip, p); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := itinerary.ResolveDayDates(&trip); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta, err := s.store.GetMeta(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta.SchemaVersion = trip.SchemaVersion
	meta.UpdatedAt = time.Now().UTC()
	res, status, err := s.commitMutate(r.Context(), id, outYAML, &meta)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	s.finishIdempotent(w, r, http.StatusOK, res)
}

func (s *Server) ListVersions(w http.ResponseWriter, r *http.Request, id string) {
	if !s.requireTripID(w, id) {
		return
	}
	vers, err := s.store.ListVersions(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "versions": vers})
}

func (s *Server) RestoreVersion(w http.ResponseWriter, r *http.Request, id string, _ api.RestoreVersionParams) {
	if err := s.requireIdempotency(w, r); err != nil {
		return
	}
	if !s.requireTripID(w, id) {
		return
	}
	body, err := s.readBody(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var req struct {
		VersionID string `json:"version_id"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.VersionID == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("version_id required"))
		return
	}
	obj, err := s.store.GetYAMLVersion(r.Context(), id, req.VersionID)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	trip, err := prepareTripYAML(obj.Body)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	outYAML, err := itinerary.MarshalYAML(trip)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta, err := s.store.GetMeta(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta.SchemaVersion = trip.SchemaVersion
	meta.UpdatedAt = time.Now().UTC()
	res, status, err := s.commitMutate(r.Context(), id, outYAML, &meta)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	s.finishIdempotent(w, r, http.StatusOK, res)
}

func (s *Server) commitMutate(ctx context.Context, id string, yamlBytes []byte, meta *store.Meta) (mutateResult, int, error) {
	vid, err := s.store.PutYAML(ctx, id, yamlBytes)
	if err != nil {
		return mutateResult{}, http.StatusInternalServerError, err
	}
	if err := s.store.PutMeta(ctx, id, *meta); err != nil {
		return mutateResult{}, http.StatusInternalServerError, err
	}

	trip, err := itinerary.ParseYAML(yamlBytes)
	if err != nil {
		return mutateResult{}, http.StatusInternalServerError, err
	}
	_ = itinerary.ResolveDayDates(&trip)
	_ = itinerary.ResolvePlaces(&trip)

	res := mutateResult{
		ID:            id,
		VersionID:     vid,
		SchemaVersion: trip.SchemaVersion,
	}
	base := s.cfg.PublicBaseURL
	if base != "" {
		res.ViewerURL = fmt.Sprintf("%s/me/trips/%s/", base, id)
	} else {
		res.ViewerURL = fmt.Sprintf("/me/trips/%s/", id)
	}

	if err := s.regenBundle(ctx, id, trip); err != nil {
		res.BundleOK = false
		res.BundleError = err.Error()
	} else {
		res.BundleOK = true
	}
	return res, http.StatusOK, nil
}

func (s *Server) regenBundle(ctx context.Context, id string, trip itinerary.Trip) error {
	dir, err := os.MkdirTemp("", "tripmap-bundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	opts := routebuild.RouteOptions{
		Mode:           s.cfg.RouteMode,
		SimplifyMeters: 100,
		CoordPrecision: 5,
		Units:          "km",
	}
	if opts.Mode == "" {
		opts.Mode = "osrm"
	}
	// For tests / offline, allow straight without OSRM.
	if opts.Mode == "osrm" && os.Getenv("TRIPMAP_FORCE_STRAIGHT") == "1" {
		opts.Mode = "straight"
	}
	if err := bundle.Build(ctx, trip, id, "", dir, opts); err != nil {
		// fall back to straight if OSRM fails
		if opts.Mode != "straight" {
			opts.Mode = "straight"
			if err2 := bundle.Build(ctx, trip, id, "", dir, opts); err2 != nil {
				return err
			}
		} else {
			return err
		}
	}
	return s.store.UploadBundle(ctx, id, dir)
}

func prepareTripYAML(b []byte) (itinerary.Trip, error) {
	trip, err := itinerary.ParseYAML(b)
	if err != nil {
		return itinerary.Trip{}, err
	}
	if err := itinerary.EnsureSchemaVersion(&trip); err != nil {
		return itinerary.Trip{}, err
	}
	if err := itinerary.ValidateBasic(trip); err != nil {
		return itinerary.Trip{}, err
	}
	if err := itinerary.ResolvePlaces(&trip); err != nil {
		return itinerary.Trip{}, err
	}
	if err := itinerary.ResolveDayDates(&trip); err != nil {
		return itinerary.Trip{}, err
	}
	return trip, nil
}

func parseCreateBody(body []byte) (id string, yamlBytes []byte, err error) {
	trim := strings.TrimSpace(string(body))
	if strings.HasPrefix(trim, "{") {
		var req struct {
			ID   string `json:"id"`
			YAML string `json:"yaml"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			return "", nil, err
		}
		if req.ID == "" || req.YAML == "" {
			return "", nil, fmt.Errorf("json body requires id and yaml")
		}
		return req.ID, []byte(req.YAML), nil
	}
	// YAML body with optional X-Trip-Id handled by caller — require JSON for create
	return "", nil, fmt.Errorf("POST /trips expects JSON {\"id\",\"yaml\"}")
}

func (s *Server) baseURL(r *http.Request) string {
	if s.cfg.PublicBaseURL != "" {
		return s.cfg.PublicBaseURL
	}
	return "https://" + r.Host
}

func (s *Server) readBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	limited := io.LimitReader(r.Body, s.cfg.MaxYAMLBytes+1)
	b, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > s.cfg.MaxYAMLBytes {
		return nil, fmt.Errorf("body exceeds MAX_YAML_BYTES (%d)", s.cfg.MaxYAMLBytes)
	}
	return b, nil
}

func (s *Server) requireTripID(w http.ResponseWriter, id string) bool {
	if err := itinerary.ValidateID(id); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

func (s *Server) requireIdempotency(w http.ResponseWriter, r *http.Request) error {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("Idempotency-Key header required"))
		return fmt.Errorf("missing idempotency key")
	}
	if err := store.ValidateIdempotencyKey(key); err != nil {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("Idempotency-Key: %w", err))
		return err
	}
	if prev, ok, err := s.store.GetIdempotency(r.Context(), key); err == nil && ok {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("X-Idempotent-Replay", "true")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(prev)
		return fmt.Errorf("replayed")
	}
	return nil
}

func (s *Server) finishIdempotent(w http.ResponseWriter, r *http.Request, status int, res mutateResult) {
	b, _ := json.Marshal(res)
	b = append(b, '\n')
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key != "" {
		if err := store.ValidateIdempotencyKey(key); err == nil {
			_ = s.store.PutIdempotency(r.Context(), key, b)
		}
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
