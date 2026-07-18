package httpserver

import (
	"net/http"
	"strings"
)

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	base := s.cfg.PublicBaseURL
	if base == "" {
		proto := r.Header.Get("X-Forwarded-Proto")
		if proto == "" {
			if r.TLS != nil {
				proto = "https"
			} else {
				// Express Mode / ALB terminate TLS; Actions need a public https base.
				proto = "https"
			}
		}
		host := r.Host
		if host == "" {
			host = "localhost:8080"
			proto = "http"
		}
		base = proto + "://" + host
	}
	doc := strings.ReplaceAll(openAPIDoc, "{{BASE_URL}}", base)
	_, _ = w.Write([]byte(doc))
}

// ChatGPT Actions wants OpenAPI 3.1.x, no parameter $refs, schemas must be an
// object, and every object schema needs properties.
const openAPIDoc = `openapi: 3.1.0
info:
  title: tripmap agent API
  version: 0.2.2
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
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/Health"
  /api/agent/schema:
    get:
      operationId: getSchema
      summary: Itinerary schema and version
      security:
        - bearerAuth: []
      responses:
        "200":
          description: Schema
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/SchemaInfo"
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
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/TripList"
        "401":
          description: Unauthorized
    post:
      operationId: createTrip
      summary: Create itinerary
      security:
        - bearerAuth: []
      parameters:
        - name: Idempotency-Key
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/CreateTripRequest"
      responses:
        "201":
          description: Created
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/MutateResult"
        "401":
          description: Unauthorized
  /api/agent/trips/{id}:
    get:
      operationId: getTrip
      summary: Trip summary
      security:
        - bearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Summary
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/TripSummary"
        "404":
          description: Not found
    patch:
      operationId: patchTrip
      summary: Structured patch
      security:
        - bearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: Idempotency-Key
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/TripPatch"
      responses:
        "200":
          description: Updated
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/MutateResult"
  /api/agent/trips/{id}/yaml:
    get:
      operationId: getTripYAML
      summary: Get raw YAML
      security:
        - bearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: YAML document
          content:
            text/plain:
              schema:
                type: string
    put:
      operationId: putTripYAML
      summary: Replace YAML (raw text body)
      security:
        - bearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: Idempotency-Key
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          text/plain:
            schema:
              type: string
              description: Full itinerary YAML
      responses:
        "200":
          description: Updated
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/MutateResult"
  /api/agent/trips/{id}/viewer-url:
    get:
      operationId: getViewerURL
      summary: Viewer URL template
      security:
        - bearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: token
          in: query
          required: false
          schema:
            type: string
      responses:
        "200":
          description: Template
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/ViewerURL"
  /api/agent/trips/{id}/rotate-token:
    post:
      operationId: rotateToken
      summary: Rotate capability token
      security:
        - bearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: Idempotency-Key
          in: header
          required: true
          schema:
            type: string
      responses:
        "200":
          description: New token
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/MutateResult"
  /api/agent/trips/{id}/versions:
    get:
      operationId: listVersions
      summary: List YAML versions
      security:
        - bearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: Versions
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/VersionList"
  /api/agent/trips/{id}/restore:
    post:
      operationId: restoreVersion
      summary: Restore a prior YAML version
      security:
        - bearerAuth: []
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
        - name: Idempotency-Key
          in: header
          required: true
          schema:
            type: string
      requestBody:
        required: true
        content:
          application/json:
            schema:
              $ref: "#/components/schemas/RestoreRequest"
      responses:
        "200":
          description: Restored
          content:
            application/json:
              schema:
                $ref: "#/components/schemas/MutateResult"
components:
  schemas:
    Health:
      type: object
      properties:
        status:
          type: string
    SchemaInfo:
      type: object
      properties:
        schema_version:
          type: integer
        description:
          type: string
        fields:
          type: object
          additionalProperties:
            type: string
        patch_ops:
          type: array
          items:
            type: string
    TripList:
      type: object
      properties:
        trips:
          type: array
          items:
            type: string
    CreateTripRequest:
      type: object
      required:
        - id
        - yaml
      properties:
        id:
          type: string
        yaml:
          type: string
    TripSummary:
      type: object
      properties:
        id:
          type: string
        version_id:
          type: string
        schema_version:
          type: integer
        trip:
          type: string
        description:
          type: string
        start:
          type: string
        days:
          type: integer
    TripPatch:
      type: object
      properties:
        swap_days:
          type: array
          items:
            type: integer
          minItems: 2
          maxItems: 2
        days:
          type: object
          additionalProperties:
            type: object
            properties:
              title:
                type: string
              notes:
                type: string
              hike:
                type: boolean
              ferry:
                type: boolean
        delete_day:
          type: integer
        insert_day:
          type: object
          properties:
            after:
              type: integer
            day:
              type: object
              properties:
                title:
                  type: string
                notes:
                  type: string
    MutateResult:
      type: object
      properties:
        id:
          type: string
        version_id:
          type: string
        schema_version:
          type: integer
        viewer_url:
          type: string
        token:
          type: string
        bundle_ok:
          type: boolean
        bundle_error:
          type: string
    ViewerURL:
      type: object
      properties:
        id:
          type: string
        base_url:
          type: string
        path_template:
          type: string
        note:
          type: string
        viewer_url:
          type: string
    VersionList:
      type: object
      properties:
        id:
          type: string
        versions:
          type: array
          items:
            type: object
            properties:
              version_id:
                type: string
              last_modified:
                type: string
              is_latest:
                type: boolean
    RestoreRequest:
      type: object
      required:
        - version_id
      properties:
        version_id:
          type: string
  securitySchemes:
    bearerAuth:
      type: http
      scheme: bearer
`
