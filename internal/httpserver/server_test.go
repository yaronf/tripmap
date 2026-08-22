package httpserver

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/yaronf/tripmap/internal/store"
)

func testServer(t *testing.T) (*Server, *store.Mem) {
	t.Helper()
	mem := store.NewMem()
	srv := New(Config{
		AgentBearerToken: "secret",
		PublicBaseURL:    "https://example.test",
		MaxYAMLBytes:     512 * 1024,
		RouteMode:        "straight",
	}, mem)
	return srv, mem
}

func authReq(method, path, token string, body io.Reader) *http.Request {
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestHealth(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestAgentRequiresBearer(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agent/trips", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAgentRejectsUnsafeTripID(t *testing.T) {
	srv, _ := testServer(t)
	// Mux cleans ".." in the URL; still reject ids that fail ValidateID.
	for _, id := range []string{"BadID", "has_underscore", "-leading"} {
		req := authReq(http.MethodGet, "/api/agent/trips/"+id, "secret", nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("id %q: status = %d body=%s, want 400", id, rec.Code, rec.Body.String())
		}
	}
}

func TestAgentRejectsUnsafeIdempotencyKey(t *testing.T) {
	srv, _ := testServer(t)
	body := []byte(`{"id":"safe-trip","yaml":"schema_version: 2\ntrip: T\nplaces:\n  a:\n    title: A\n    lat: 1\n    lon: 2\ndays:\n  - day: 1\n    title: D\n    stops:\n      - {place: a}\n"}`)
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "../../trips/holland/itinerary.yaml")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", rec.Code, rec.Body.String())
	}
}

func TestOpenAPIPublic(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body, _ := io.ReadAll(rec.Body)
	if !strings.Contains(string(body), "https://example.test") {
		t.Fatalf("missing base url in openapi")
	}
	if !strings.Contains(string(body), "putTripYAML") {
		t.Fatalf("missing putTripYAML in openapi")
	}
}

func TestMCPRequiresBearer(t *testing.T) {
	srv, _ := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestMCPListTripsTool(t *testing.T) {
	srv, mem := testServer(t)
	if _, err := mem.PutYAML(t.Context(), "holland", []byte(sampleYAML)); err != nil {
		t.Fatal(err)
	}
	h := srv.Handler()

	mcpCall := func(body string) string {
		t.Helper()
		req := authReq(http.MethodPost, "/mcp", "secret", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		raw := rec.Body.String()
		if strings.Contains(raw, "data:") {
			var parts []string
			for _, line := range strings.Split(raw, "\n") {
				if strings.HasPrefix(line, "data:") {
					parts = append(parts, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
				}
			}
			return strings.Join(parts, "\n")
		}
		return raw
	}

	init := mcpCall(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`)
	if !strings.Contains(init, `"name":"tripmap"`) {
		t.Fatalf("initialize: %s", init)
	}
	list := mcpCall(`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{}}`)
	if !strings.Contains(list, `"name":"listTrips"`) {
		t.Fatalf("tools/list: %s", list)
	}
	call := mcpCall(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"listTrips","arguments":{}}}`)
	if !strings.Contains(call, "holland") {
		t.Fatalf("listTrips: %s", call)
	}
}

func TestOpenAPIUsesRequestHost(t *testing.T) {
	mem := store.NewMem()
	srv := New(Config{AgentBearerToken: "secret", MaxYAMLBytes: 512 * 1024, RouteMode: "straight"}, mem)
	req := httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil)
	req.Host = "tr-example.ecs.eu-central-1.on.aws"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "https://tr-example.ecs.eu-central-1.on.aws") {
		t.Fatalf("expected host-derived servers.url, got %s", body[:min(200, len(body))])
	}
}

func TestLoadConfigFromJSONSecret(t *testing.T) {
	t.Setenv("AGENT_BEARER_TOKEN", "")
	t.Setenv("AGENT_BEARER_SECRET_JSON", `{"token":"from-json"}`)
	t.Setenv("DESCOPE_PROJECT_ID", "")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_SECRET_JSON", "")
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AgentBearerToken != "from-json" {
		t.Fatalf("token = %q", cfg.AgentBearerToken)
	}
}

func TestLoadConfigRequiresToken(t *testing.T) {
	t.Setenv("AGENT_BEARER_TOKEN", "")
	t.Setenv("AGENT_BEARER_SECRET_JSON", "")
	t.Setenv("DESCOPE_PROJECT_ID", "")
	t.Setenv("SESSION_SECRET", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_SECRET_JSON", "")
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected error")
	}
}

const sampleYAML = `trip: Smoke Trip
description: test
places:
  a:
    title: A
    lat: 1.0
    lon: 2.0
    type: overnight
days:
  - day: 1
    title: Start
    stops:
      - { place: a }
`

func TestCreatePutGetIdempotentPatch(t *testing.T) {
	srv, mem := testServer(t)

	createBody, _ := json.Marshal(map[string]string{"id": "smoke-trip", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "create-1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", rec.Code, rec.Body.String())
	}
	var created mutateResult
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ViewerURL == "" || !strings.Contains(created.ViewerURL, "/me/trips/smoke-trip/") || !created.BundleOK {
		t.Fatalf("create result = %+v", created)
	}
	if files := mem.BundleFiles("smoke-trip"); len(files) == 0 || files["trip.json"] == nil {
		t.Fatalf("bundle not uploaded: %v", files)
	}

	// idempotent replay
	req2 := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req2.Header.Set("Idempotency-Key", "create-1")
	req2.Header.Set("Content-Type", "application/json")
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req2)
	if rec2.Header().Get("X-Idempotent-Replay") != "true" {
		t.Fatalf("expected replay header, body=%s", rec2.Body.String())
	}

	// list
	req = authReq(http.MethodGet, "/api/agent/trips", "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "smoke-trip") {
		t.Fatalf("list = %s", rec.Body.String())
	}

	// get yaml
	req = authReq(http.MethodGet, "/api/agent/trips/smoke-trip/yaml", "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Smoke Trip") {
		t.Fatalf("yaml = %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "schema_version") {
		t.Fatalf("expected schema_version injected: %s", rec.Body.String())
	}

	// put yaml
	putYAML := sampleYAML + "\n# updated\n"
	req = authReq(http.MethodPut, "/api/agent/trips/smoke-trip/yaml", "secret", strings.NewReader(putYAML))
	req.Header.Set("Idempotency-Key", "put-1")
	req.Header.Set("Content-Type", "application/yaml")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("put status=%d body=%s", rec.Code, rec.Body.String())
	}

	// patch swap (single day — just update title)
	patch := `{"days":{"1":{"title":"Renamed"}}}`
	req = authReq(http.MethodPatch, "/api/agent/trips/smoke-trip", "secret", strings.NewReader(patch))
	req.Header.Set("Idempotency-Key", "patch-1")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/agent/trips/smoke-trip/yaml", "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Renamed") {
		t.Fatalf("after patch yaml=%s", rec.Body.String())
	}

	// versions
	req = authReq(http.MethodGet, "/api/agent/trips/smoke-trip/versions", "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "version_id") {
		t.Fatalf("versions = %s", rec.Body.String())
	}
	var versPayload struct {
		Versions []struct {
			VersionID string `json:"version_id"`
		} `json:"versions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &versPayload); err != nil || len(versPayload.Versions) == 0 {
		t.Fatalf("parse versions: %v body=%s", err, rec.Body.String())
	}
	vid := versPayload.Versions[0].VersionID
	req = authReq(http.MethodGet, "/api/agent/trips/smoke-trip/versions/"+vid, "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "trip:") {
		t.Fatalf("getVersion status=%d body=%s", rec.Code, rec.Body.String())
	}

	// schema
	req = authReq(http.MethodGet, "/api/agent/schema", "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"schema_version":2`) {
		t.Fatalf("schema = %s", rec.Body.String())
	}

	// put without idempotency key
	req = authReq(http.MethodPut, "/api/agent/trips/smoke-trip/yaml", "secret", strings.NewReader(sampleYAML))
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("want 400 without idempotency, got %d", rec.Code)
	}
}

func TestRejectBadSchemaVersion(t *testing.T) {
	srv, _ := testServer(t)
	bad := "schema_version: 99\ntrip: X\ndays:\n  - {day: 1, title: A, stops: [{name: A, lat: 1, lon: 2}]}\n"
	body, _ := json.Marshal(map[string]string{"id": "bad-schema", "yaml": bad})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "bad-1")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestPatchSwapDays(t *testing.T) {
	srv, _ := testServer(t)
	yaml := `trip: Swap
places:
  a: { title: A, lat: 1, lon: 2, type: overnight }
  b: { title: B, lat: 3, lon: 4, type: overnight }
days:
  - day: 1
    title: One
    stops: [{ place: a }]
  - day: 2
    title: Two
    stops: [{ place: b }]
`
	body, _ := json.Marshal(map[string]string{"id": "swap-trip", "yaml": yaml})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(body))
	req.Header.Set("Idempotency-Key", "swap-c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}

	req = authReq(http.MethodPatch, "/api/agent/trips/swap-trip", "secret", strings.NewReader(`{"swap_days":[1,2]}`))
	req.Header.Set("Idempotency-Key", "swap-p")
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("patch: %s", rec.Body.String())
	}

	req = authReq(http.MethodGet, "/api/agent/trips/swap-trip/yaml", "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	got := rec.Body.String()
	// After swap, day 1 should be titled Two
	if !strings.Contains(got, "title: Two") {
		t.Fatalf("expected swapped titles in %s", got)
	}
}

