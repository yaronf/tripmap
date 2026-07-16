package httpserver

import (
	"net/http"
	"strings"
)

func (s *Server) handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	base := s.cfg.PublicBaseURL
	if base == "" {
		base = "http://localhost:8080"
	}
	doc := strings.ReplaceAll(openAPIDoc, "{{BASE_URL}}", base)
	_, _ = w.Write([]byte(doc))
}

const openAPIDoc = `openapi: 3.1.0
info:
  title: tripmap agent API
  version: 0.2.0
  description: Authenticated itinerary API for Custom GPT Actions.
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
  /api/agent/schema:
    get:
      operationId: getSchema
      summary: Itinerary schema and version
      security:
        - bearerAuth: []
      responses:
        "200":
          description: Schema
        "401":
          description: Unauthorized
  /api/agent/trips:
    get:
      operationId: listTrips
      summary: List itinerary IDs
      security:
        - bearerAuth: []
      responses:
        "200":
          description: Trip ID list
        "401":
          description: Unauthorized
    post:
      operationId: createTrip
      summary: Create itinerary
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/IdempotencyKey"
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [id, yaml]
              properties:
                id:
                  type: string
                yaml:
                  type: string
      responses:
        "201":
          description: Created
        "401":
          description: Unauthorized
  /api/agent/trips/{id}:
    get:
      operationId: getTrip
      summary: Trip summary
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/TripId"
      responses:
        "200":
          description: Summary
        "404":
          description: Not found
    patch:
      operationId: patchTrip
      summary: Structured patch
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/TripId"
        - $ref: "#/components/parameters/IdempotencyKey"
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
      responses:
        "200":
          description: Updated
  /api/agent/trips/{id}/yaml:
    get:
      operationId: getTripYAML
      summary: Get raw YAML
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/TripId"
      responses:
        "200":
          description: YAML
    put:
      operationId: putTripYAML
      summary: Replace YAML
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/TripId"
        - $ref: "#/components/parameters/IdempotencyKey"
      requestBody:
        required: true
        content:
          application/yaml:
            schema:
              type: string
      responses:
        "200":
          description: Updated
  /api/agent/trips/{id}/viewer-url:
    get:
      operationId: getViewerURL
      summary: Viewer URL template (token only with ?token=)
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/TripId"
      responses:
        "200":
          description: Template
  /api/agent/trips/{id}/rotate-token:
    post:
      operationId: rotateToken
      summary: Rotate capability token
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/TripId"
        - $ref: "#/components/parameters/IdempotencyKey"
      responses:
        "200":
          description: New token
  /api/agent/trips/{id}/versions:
    get:
      operationId: listVersions
      summary: List YAML versions
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/TripId"
      responses:
        "200":
          description: Versions
  /api/agent/trips/{id}/restore:
    post:
      operationId: restoreVersion
      summary: Restore a prior YAML version
      security:
        - bearerAuth: []
      parameters:
        - $ref: "#/components/parameters/TripId"
        - $ref: "#/components/parameters/IdempotencyKey"
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [version_id]
              properties:
                version_id:
                  type: string
      responses:
        "200":
          description: Restored
components:
  parameters:
    TripId:
      name: id
      in: path
      required: true
      schema:
        type: string
    IdempotencyKey:
      name: Idempotency-Key
      in: header
      required: true
      schema:
        type: string
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
`
