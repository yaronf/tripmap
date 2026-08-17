package httpserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignedInRootListsTrips(t *testing.T) {
	srv, _ := testServer(t)
	srv.cfg.HelloClientID = "app_test"
	srv.cfg.HelloSessionSecret = "session-test-key"
	srv.cfg.HelloAllowedEmails = []string{"a@b.c"}

	createBody, _ := json.Marshal(map[string]string{"id": "list-trip", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "list-c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	sess := sessionCookie{Sub: "user-1", Email: "a@b.c", Name: "Ada", Exp: time.Now().Add(time.Hour).Unix()}
	if err := srv.setSignedCookie(rec, req, sessionCookieName, sess, time.Hour); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `/me/trips/list-trip/`) {
		t.Fatalf("missing trip link: %s", body)
	}
	if !strings.Contains(body, "Smoke Trip") && !strings.Contains(body, "list-trip") {
		t.Fatalf("missing title: %s", body)
	}
}

func TestSessionTripRequiresLogin(t *testing.T) {
	srv, _ := testServer(t)
	srv.cfg.HelloClientID = "app_test"
	srv.cfg.HelloSessionSecret = "session-test-key"
	srv.cfg.HelloAllowedEmails = []string{"a@b.c"}

	req := httptest.NewRequest(http.MethodGet, "/me/trips/x/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("code=%d want 302", rec.Code)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/auth/hello/login") || !strings.Contains(loc, "return_to") {
		t.Fatalf("location=%s", loc)
	}
}

func TestSessionTripManifestPublicWithoutCookie(t *testing.T) {
	srv, _ := testServer(t)
	srv.cfg.HelloClientID = "app_test"
	srv.cfg.HelloSessionSecret = "session-test-key"
	srv.cfg.HelloAllowedEmails = []string{"a@b.c"}

	createBody, _ := json.Marshal(map[string]string{"id": "manifest-trip", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "manifest-c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/me/trips/manifest-trip/manifest.webmanifest", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "manifest") && !strings.Contains(ct, "json") {
		t.Fatalf("content-type=%s", ct)
	}
	if !strings.Contains(rec.Body.String(), `"name"`) {
		t.Fatalf("body=%s", rec.Body.String())
	}

	// Still require login for the trip shell itself.
	req = httptest.NewRequest(http.MethodGet, "/me/trips/manifest-trip/", nil)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("index code=%d want 302", rec.Code)
	}
}

func TestSessionTripServesBundle(t *testing.T) {
	srv, _ := testServer(t)
	srv.cfg.HelloClientID = "app_test"
	srv.cfg.HelloSessionSecret = "session-test-key"
	srv.cfg.HelloAllowedEmails = []string{"a@b.c"}

	createBody, _ := json.Marshal(map[string]string{"id": "sess-trip", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "sess-c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	sess := sessionCookie{Sub: "user-1", Email: "a@b.c", Name: "Ada", Exp: time.Now().Add(time.Hour).Unix()}
	if err := srv.setSignedCookie(rec, req, sessionCookieName, sess, time.Hour); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]

	req = httptest.NewRequest(http.MethodGet, "/me/trips/sess-trip/", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "<title>") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String()[:min(200, rec.Body.Len())])
	}

	base := "/me/trips/sess-trip/"
	req = httptest.NewRequest(http.MethodGet, base+"api/notes", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"days"`) {
		t.Fatalf("notes get: %s", rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, base+"api/notes", strings.NewReader(`{"days":{"1":"hi"}}`))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hi") {
		t.Fatalf("notes put: %s", rec.Body.String())
	}
}

func TestSessionTripViewerComesFromImageNotS3(t *testing.T) {
	srv, mem := testServer(t)
	srv.cfg.HelloClientID = "app_test"
	srv.cfg.HelloSessionSecret = "session-test-key"
	srv.cfg.HelloAllowedEmails = []string{"a@b.c"}

	createBody, _ := json.Marshal(map[string]string{"id": "embed-trip", "yaml": sampleYAML})
	req := authReq(http.MethodPost, "/api/agent/trips", "secret", bytes.NewReader(createBody))
	req.Header.Set("Idempotency-Key", "embed-c")
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %s", rec.Body.String())
	}

	mem.PutBundleFile("embed-trip", "app.js", []byte("/* stale s3 copy */"))

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	sess := sessionCookie{Sub: "user-1", Email: "a@b.c", Name: "Ada", Exp: time.Now().Add(time.Hour).Unix()}
	if err := srv.setSignedCookie(rec, req, sessionCookieName, sess, time.Hour); err != nil {
		t.Fatal(err)
	}
	cookie := rec.Result().Cookies()[0]

	req = httptest.NewRequest(http.MethodGet, "/me/trips/embed-trip/app.js", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "stale s3 copy") {
		t.Fatal("served S3 app.js instead of the image embed")
	}
	if !strings.Contains(rec.Body.String(), "SINGLE_LOCATION_ZOOM") {
		t.Fatalf("missing embedded viewer: %s", rec.Body.String()[:min(200, rec.Body.Len())])
	}
}
