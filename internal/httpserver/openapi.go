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
  version: 0.3.0
  description: |
    Authenticated itinerary API for Custom GPT Actions.

    Schema (version 2): each trip has a places catalog (stable ids → title,
    lat, lon, type, optional info) and days whose route/stops are place refs
    ({place, type?, notes?}).

    Notes policy: day notes and stop notes are human-authored. Do not modify
    them and do not put links, trail stats, or other machine enrichment into
    notes unless the user explicitly asks you to edit their notes. Put
    enrichment under places.<id>.info instead. The API still accepts notes on
    PATCH/PUT; this is a usage rule, not a server check.
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
      summary: Structured patch (places enrichment, day fields, swap/insert/delete)
      description: |
        Prefer this over putTripYAML for small edits.

        Enrichment — patch places by stable id (deep-merges info; replaces
        links/warnings/highlights arrays when sent):

          {"places":{"tongariro-crossing":{"info":{"links":[{"type":"alltrails","title":"AllTrails","url":"https://example.com"}],"warnings":["Alpine weather"]}}}}

        Day itinerary — rename etc. (do not put enrichment in notes):

          {"days":{"8":{"title":"New title"}}}

        Also supports upsert_stop / remove_stop (by place id), swap_days,
        insert_day, delete_day, and day fields hike/ferry.
        Use putTripYAML only for large rewrites.
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
        notes_policy:
          type: string
          description: Human-authored notes usage rule for agents (not enforced)
        fields:
          type: object
          additionalProperties:
            type: string
        patch_ops:
          type: array
          items:
            type: string
        days_patch_example:
          type: object
          additionalProperties: true
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
      description: |
        One or more structured ops. Prefer places.<id>.info for enrichment.
        Day keys are strings matching the day number.
      properties:
        swap_days:
          type: array
          description: Exactly two day numbers to swap
          items:
            type: integer
          minItems: 2
          maxItems: 2
        places:
          type: object
          description: Map of place id to partial place fields (info is deep-merged)
          additionalProperties:
            type: object
            properties:
              title:
                type: string
              lat:
                type: number
              lon:
                type: number
              type:
                type: string
              info:
                $ref: "#/components/schemas/PlaceInfo"
        days:
          type: object
          description: Map of day-number string to partial day fields to merge
          additionalProperties:
            type: object
            properties:
              title:
                type: string
              notes:
                type: string
                description: |
                  Human-authored day notes. Full replace if sent. Agents should
                  not modify notes unless the user explicitly asks.
              hike:
                type: boolean
              ferry:
                type: boolean
        upsert_stop:
          type: object
          properties:
            day:
              type: integer
            list:
              type: string
              description: route or stops
            place:
              type: string
            type:
              type: string
            notes:
              type: string
        remove_stop:
          type: object
          properties:
            day:
              type: integer
            list:
              type: string
              description: route, stops, or empty for both
            place:
              type: string
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
                  description: Human-authored; agents should not set unless asked
                hike:
                  type: boolean
                ferry:
                  type: boolean
                route:
                  type: array
                  items:
                    $ref: "#/components/schemas/StopRef"
                stops:
                  type: array
                  items:
                    $ref: "#/components/schemas/StopRef"
    PlaceInfo:
      type: object
      description: Structured enrichment for a place (optional throughout)
      properties:
        source:
          type: object
          properties:
            generated_by:
              type: string
            generated_at:
              type: string
        links:
          type: array
          items:
            type: object
            properties:
              type:
                type: string
              title:
                type: string
              url:
                type: string
            required:
              - type
              - url
        stats:
          type: object
          properties:
            distance_km:
              type: number
            duration:
              type: string
            ascent_m:
              type: number
            difficulty:
              type: string
        logistics:
          type: object
          properties:
            parking:
              type: string
            booking_required:
              type: boolean
        facilities:
          type: object
          properties:
            toilets:
              type: boolean
            drinking_water:
              type: boolean
        warnings:
          type: array
          items:
            type: string
        highlights:
          type: array
          items:
            type: string
    StopRef:
      type: object
      properties:
        place:
          type: string
        type:
          type: string
        notes:
          type: string
      required:
        - place
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
