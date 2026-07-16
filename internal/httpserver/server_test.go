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

func TestLoadConfigFromJSONSecret(t *testing.T) {
	t.Setenv("AGENT_BEARER_TOKEN", "")
	t.Setenv("AGENT_BEARER_SECRET_JSON", `{"token":"from-json"}`)
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
	if _, err := LoadConfig(); err == nil {
		t.Fatal("expected error")
	}
}

const sampleYAML = `trip: Smoke Trip
description: test
days:
  - day: 1
    title: Start
    stops:
      - { name: A, type: overnight, lat: 1.0, lon: 2.0 }
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
	if created.Token == "" || created.ViewerURL == "" || !created.BundleOK {
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

	// rotate
	req = authReq(http.MethodPost, "/api/agent/trips/smoke-trip/rotate-token", "secret", nil)
	req.Header.Set("Idempotency-Key", "rot-1")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"token"`) {
		t.Fatalf("rotate = %s", rec.Body.String())
	}

	// versions
	req = authReq(http.MethodGet, "/api/agent/trips/smoke-trip/versions", "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "version_id") {
		t.Fatalf("versions = %s", rec.Body.String())
	}

	// schema
	req = authReq(http.MethodGet, "/api/agent/schema", "secret", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"schema_version":1`) {
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
days:
  - day: 1
    title: One
    stops: [{name: A, type: overnight, lat: 1, lon: 2}]
  - day: 2
    title: Two
    stops: [{name: B, type: overnight, lat: 3, lon: 4}]
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

func TestCapabilityBundleAndNotes(t *testing.T) {
	srv, _ := testServer(t)
	createBody, _ := json.Marshal(map[string]string{"id": "cap-trip", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "cap-c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}
	var created mutateResult
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	bad := httptest.NewRequest(http.MethodGet, "/t/cap-trip/wrong-token/index.html", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, bad)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad token status=%d", rec.Code)
	}

	base := "/t/cap-trip/" + created.Token + "/"
	req = httptest.NewRequest(http.MethodGet, base+"index.html", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>") {
		t.Fatalf("index status=%d body=%s", rec.Code, rec.Body.String()[:min(200, rec.Body.Len())])
	}

	req = httptest.NewRequest(http.MethodGet, base+"api/notes", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"days"`) {
		t.Fatalf("notes get: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, base+"api/notes", strings.NewReader(`{"days":{"1":"hi"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hi") {
		t.Fatalf("notes put: %s", rec.Body.String())
	}

	// redirect without trailing slash
	req = httptest.NewRequest(http.MethodGet, "/t/cap-trip/"+created.Token, nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("redirect status=%d", rec.Code)
	}
}
