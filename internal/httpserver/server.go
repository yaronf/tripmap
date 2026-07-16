package httpserver

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Server is the tripmapd HTTP API.
type Server struct {
	cfg Config
	mux *http.ServeMux
}

// New builds the HTTP server with Phase A routes.
func New(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}
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
	agent.HandleFunc("GET /trips", s.handleAgentNotImplemented)
	agent.HandleFunc("POST /trips", s.handleAgentNotImplemented)
	agent.HandleFunc("GET /schema", s.handleAgentNotImplemented)
	agent.HandleFunc("GET /trips/{id}", s.handleAgentNotImplemented)
	agent.HandleFunc("GET /trips/{id}/yaml", s.handleAgentNotImplemented)
	agent.HandleFunc("PUT /trips/{id}/yaml", s.handleAgentNotImplemented)
	agent.HandleFunc("PATCH /trips/{id}", s.handleAgentNotImplemented)
	agent.HandleFunc("GET /trips/{id}/viewer-url", s.handleAgentNotImplemented)
	agent.HandleFunc("POST /trips/{id}/rotate-token", s.handleAgentNotImplemented)
	agent.HandleFunc("GET /trips/{id}/versions", s.handleAgentNotImplemented)
	agent.HandleFunc("POST /trips/{id}/restore", s.handleAgentNotImplemented)

	s.mux.Handle("/api/agent/", http.StripPrefix("/api/agent", bearerAuth(s.cfg.AgentBearerToken, agent)))
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleRoot(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("tripmapd\n"))
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	base := s.cfg.PublicBaseURL
	if base == "" {
		base = "http://localhost:8080"
	}
	doc := strings.ReplaceAll(openAPIStub, "{{BASE_URL}}", base)
	_, _ = w.Write([]byte(doc))
}

func (s *Server) handleAgentNotImplemented(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"error": "not_implemented",
		"hint":  "Phase B will wire S3 itinerary APIs",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

const openAPIStub = `openapi: 3.1.0
info:
  title: tripmap agent API
  version: 0.1.0-phase-a
  description: Authenticated itinerary API for Custom GPT Actions (stub until Phase B).
servers:
  - url: {{BASE_URL}}
paths:
  /health:
    get:
      operationId: health
      summary: Liveness
      security: []
      responses:
        "200":
          description: OK
  /api/agent/trips:
    get:
      operationId: listTrips
      summary: List itinerary IDs
      security:
        - bearerAuth: []
      responses:
        "401":
          description: Unauthorized
        "501":
          description: Not implemented yet
    post:
      operationId: createTrip
      summary: Create itinerary
      security:
        - bearerAuth: []
      responses:
        "401":
          description: Unauthorized
        "501":
          description: Not implemented yet
  /api/agent/schema:
    get:
      operationId: getSchema
      summary: Itinerary JSON Schema
      security:
        - bearerAuth: []
      responses:
        "401":
          description: Unauthorized
        "501":
          description: Not implemented yet
components:
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
`
