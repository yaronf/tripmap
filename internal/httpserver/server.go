package httpserver

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yaronf/tripmap/internal/bundle"
	"github.com/yaronf/tripmap/internal/itinerary"
	"github.com/yaronf/tripmap/internal/routebuild"
	"github.com/yaronf/tripmap/internal/store"
)

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
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /openapi.yaml", s.handleOpenAPI)
	s.mux.HandleFunc("GET /{$}", s.handleRoot)

	agent := http.NewServeMux()
	agent.HandleFunc("GET /trips", s.handleListTrips)
	agent.HandleFunc("POST /trips", s.handleCreateTrip)
	agent.HandleFunc("GET /schema", s.handleSchema)
	agent.HandleFunc("GET /trips/{id}", s.handleGetTrip)
	agent.HandleFunc("GET /trips/{id}/yaml", s.handleGetYAML)
	agent.HandleFunc("PUT /trips/{id}/yaml", s.handlePutYAML)
	agent.HandleFunc("PATCH /trips/{id}", s.handlePatchTrip)
	agent.HandleFunc("GET /trips/{id}/viewer-url", s.handleViewerURL)
	agent.HandleFunc("POST /trips/{id}/rotate-token", s.handleRotateToken)
	agent.HandleFunc("GET /trips/{id}/versions", s.handleListVersions)
	agent.HandleFunc("POST /trips/{id}/restore", s.handleRestore)

	s.mux.Handle("/api/agent/", http.StripPrefix("/api/agent", bearerAuth(s.cfg.AgentBearerToken, agent)))
}

type mutateResult struct {
	ID            string `json:"id"`
	VersionID     string `json:"version_id,omitempty"`
	SchemaVersion int    `json:"schema_version"`
	ViewerURL     string `json:"viewer_url,omitempty"`
	Token         string `json:"token,omitempty"`
	BundleOK      bool   `json:"bundle_ok"`
	BundleError   string `json:"bundle_error,omitempty"`
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("tripmapd\n"))
}

func (s *Server) handleListTrips(w http.ResponseWriter, r *http.Request) {
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

func (s *Server) handleSchema(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version": itinerary.CurrentSchemaVersion,
		"description":    "tripmap itinerary YAML schema",
		"fields": map[string]any{
			"schema_version": "int (required on write; server injects 1 if omitted)",
			"trip":           "string title",
			"description":    "optional string",
			"start":          "optional YYYY-MM-DD",
			"days":           "array of day objects with day, title, route/stops, flags",
		},
		"patch_ops": []string{"swap_days", "days", "insert_day", "delete_day"},
	})
}

func (s *Server) handleGetTrip(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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

func (s *Server) handleGetYAML(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
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

func (s *Server) handleCreateTrip(w http.ResponseWriter, r *http.Request) {
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

	token, hash, err := mintToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now().UTC()
	meta := store.Meta{SchemaVersion: trip.SchemaVersion, TokenHash: hash, CreatedAt: now, UpdatedAt: now}

	res, status, err := s.commitMutate(r.Context(), id, outYAML, &meta, token, true)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	s.finishIdempotent(w, r, http.StatusCreated, res)
}

func (s *Server) handlePutYAML(w http.ResponseWriter, r *http.Request) {
	if err := s.requireIdempotency(w, r); err != nil {
		return
	}
	id := r.PathValue("id")
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
	res, status, err := s.commitMutate(r.Context(), id, outYAML, &meta, "", false)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	s.finishIdempotent(w, r, http.StatusOK, res)
}

func (s *Server) handlePatchTrip(w http.ResponseWriter, r *http.Request) {
	if err := s.requireIdempotency(w, r); err != nil {
		return
	}
	id := r.PathValue("id")
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
	res, status, err := s.commitMutate(r.Context(), id, outYAML, &meta, "", false)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	s.finishIdempotent(w, r, http.StatusOK, res)
}

func (s *Server) handleViewerURL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exists, err := s.store.Exists(r.Context(), id)
	if err != nil || !exists {
		writeErr(w, http.StatusNotFound, fmt.Errorf("trip %q not found", id))
		return
	}
	base := s.cfg.PublicBaseURL
	if base == "" {
		base = "https://" + r.Host
	}
	token := r.URL.Query().Get("token")
	out := map[string]any{
		"id":            id,
		"base_url":      base,
		"path_template": fmt.Sprintf("/t/%s/{token}/", id),
		"note":          "plaintext token is only returned on create and rotate-token",
	}
	if token != "" {
		out["viewer_url"] = fmt.Sprintf("%s/t/%s/%s/", base, id, token)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleRotateToken(w http.ResponseWriter, r *http.Request) {
	if err := s.requireIdempotency(w, r); err != nil {
		return
	}
	id := r.PathValue("id")
	meta, err := s.store.GetMeta(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	token, hash, err := mintToken()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	meta.TokenHash = hash
	meta.UpdatedAt = time.Now().UTC()
	if err := s.store.PutMeta(r.Context(), id, meta); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	base := s.baseURL(r)
	res := mutateResult{
		ID:            id,
		SchemaVersion: meta.SchemaVersion,
		Token:         token,
		ViewerURL:     fmt.Sprintf("%s/t/%s/%s/", base, id, token),
		BundleOK:      true,
	}
	s.finishIdempotent(w, r, http.StatusOK, res)
}

func (s *Server) handleListVersions(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	vers, err := s.store.ListVersions(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"id": id, "versions": vers})
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if err := s.requireIdempotency(w, r); err != nil {
		return
	}
	id := r.PathValue("id")
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
	res, status, err := s.commitMutate(r.Context(), id, outYAML, &meta, "", false)
	if err != nil {
		writeErr(w, status, err)
		return
	}
	s.finishIdempotent(w, r, http.StatusOK, res)
}

func (s *Server) commitMutate(ctx context.Context, id string, yamlBytes []byte, meta *store.Meta, plaintextToken string, isCreate bool) (mutateResult, int, error) {
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

	res := mutateResult{
		ID:            id,
		VersionID:     vid,
		SchemaVersion: trip.SchemaVersion,
	}
	if plaintextToken != "" {
		res.Token = plaintextToken
		base := s.cfg.PublicBaseURL
		if base == "" {
			base = ""
		}
		if base != "" {
			res.ViewerURL = fmt.Sprintf("%s/t/%s/%s/", base, id, plaintextToken)
		} else {
			res.ViewerURL = fmt.Sprintf("/t/%s/%s/", id, plaintextToken)
		}
	}

	if err := s.regenBundle(ctx, id, trip); err != nil {
		res.BundleOK = false
		res.BundleError = err.Error()
	} else {
		res.BundleOK = true
	}
	_ = isCreate
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

func mintToken() (plaintext, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plaintext = base64.RawURLEncoding.EncodeToString(b)
	sum := sha256.Sum256([]byte(plaintext))
	hash = hex.EncodeToString(sum[:])
	return plaintext, hash, nil
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

func (s *Server) requireIdempotency(w http.ResponseWriter, r *http.Request) error {
	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("Idempotency-Key header required"))
		return fmt.Errorf("missing idempotency key")
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
		_ = s.store.PutIdempotency(r.Context(), key, b)
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
