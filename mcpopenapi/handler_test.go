package mcpopenapi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaronf/mcpopenapi"
)

const sampleOpenAPI = `
openapi: 3.1.0
info:
  title: test
  version: 0.0.1
paths:
  /health:
    get:
      operationId: health
      security: []
      responses:
        "200":
          description: OK
  /api/agent/trips:
    get:
      operationId: listTrips
      summary: List trips
      security:
        - bearerAuth: []
      responses:
        "200":
          description: OK
    post:
      operationId: createTrip
      summary: Create
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
  /api/agent/trips/{id}:
    get:
      operationId: getTrip
      summary: Get trip
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
          description: OK
    patch:
      operationId: patchTrip
      summary: Patch
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
          description: OK
components:
  schemas:
    CreateTripRequest:
      type: object
      required: [id, yaml]
      properties:
        id:
          type: string
        yaml:
          type: string
    TripPatch:
      type: object
      properties:
        update_day:
          type: object
          properties:
            day:
              type: integer
            notes:
              type: string
`

func TestNewHandlerListAndCall(t *testing.T) {
	var gotMethod, gotPath, gotIdem string
	var gotBody []byte
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotIdem = r.Header.Get("Idempotency-Key")
		gotBody = nil
		if r.Body != nil {
			gotBody, _ = io.ReadAll(r.Body)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/agent/trips":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"trips":["holland"]}`))
		case r.Method == http.MethodPatch && strings.HasPrefix(r.URL.Path, "/api/agent/trips/"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"holland","bundle_ok":true}`))
		default:
			http.NotFound(w, r)
		}
	})

	h, err := mcpopenapi.NewHandler(mcpopenapi.Config{
		Name:        "test",
		Version:     "0.0.1",
		OpenAPIYAML: []byte(sampleOpenAPI),
		Upstream:    upstream,
		PathPrefix:  "/api/agent",
	})
	if err != nil {
		t.Fatal(err)
	}

	// initialize
	initResp := mcpPOST(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	if !strings.Contains(initResp, `"name":"test"`) {
		t.Fatalf("initialize: %s", initResp)
	}

	// tools/list
	listResp := mcpPOST(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if strings.Contains(listResp, `"name":"health"`) {
		t.Fatalf("health should be skipped: %s", listResp)
	}
	for _, name := range []string{"listTrips", "createTrip", "getTrip", "patchTrip"} {
		if !strings.Contains(listResp, `"name":"`+name+`"`) {
			t.Fatalf("missing tool %s in %s", name, listResp)
		}
	}

	// listTrips
	call := mcpPOST(t, h, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"listTrips","arguments":{}}}`)
	if gotMethod != http.MethodGet || gotPath != "/api/agent/trips" {
		t.Fatalf("upstream got %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(call, "holland") || strings.Contains(call, `"isError":true`) {
		t.Fatalf("listTrips call: %s", call)
	}

	// patchTrip with auto Idempotency-Key
	gotIdem = ""
	gotBody = nil
	args, _ := json.Marshal(map[string]any{
		"id": "holland",
		"update_day": map[string]any{
			"day":   1,
			"notes": "hello",
		},
	})
	call = mcpPOST(t, h, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"patchTrip","arguments":`+string(args)+`}}`)
	if gotMethod != http.MethodPatch || gotPath != "/api/agent/trips/holland" {
		t.Fatalf("patch upstream %s %s", gotMethod, gotPath)
	}
	if gotIdem == "" {
		t.Fatal("expected auto Idempotency-Key")
	}
	if !bytes.Contains(gotBody, []byte(`"notes":"hello"`)) {
		t.Fatalf("body=%s", gotBody)
	}
	if strings.Contains(call, `"isError":true`) {
		t.Fatalf("patchTrip error: %s", call)
	}
}

func TestCreateTripBodyAndIdempotency(t *testing.T) {
	var gotIdem string
	var gotBody []byte
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIdem = r.Header.Get("Idempotency-Key")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"x"}`))
	})
	h, err := mcpopenapi.NewHandler(mcpopenapi.Config{
		Name:        "test",
		Version:     "0.0.1",
		OpenAPIYAML: []byte(sampleOpenAPI),
		Upstream:    upstream,
		PathPrefix:  "/api/agent",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = mcpPOST(t, h, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)

	args, _ := json.Marshal(map[string]any{"id": "x", "yaml": "trip: X\n"})
	resp := mcpPOST(t, h, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"createTrip","arguments":`+string(args)+`}}`)
	if gotIdem == "" {
		t.Fatal("missing Idempotency-Key")
	}
	var body map[string]any
	if err := json.Unmarshal(gotBody, &body); err != nil {
		t.Fatal(err)
	}
	if body["id"] != "x" || body["yaml"] == nil {
		t.Fatalf("body=%s", gotBody)
	}
	if strings.Contains(resp, `"isError":true`) {
		t.Fatalf("createTrip: %s", resp)
	}
}

func mcpPOST(t *testing.T, h http.Handler, body string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "http://example/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	raw := rec.Body.String()
	// Stateless SSE or JSON — extract JSON-RPC payload text.
	if strings.Contains(raw, "event:") || strings.Contains(raw, "data:") {
		var dataLines []string
		for _, line := range strings.Split(raw, "\n") {
			if strings.HasPrefix(line, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			}
		}
		return strings.Join(dataLines, "\n")
	}
	return raw
}
