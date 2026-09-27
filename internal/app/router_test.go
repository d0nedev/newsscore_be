package app

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/d0nedev/newsscore/internal/platform/config"
	"github.com/d0nedev/newsscore/internal/platform/health"

	"github.com/go-chi/chi/v5"
)

func testRouter(t *testing.T, logs *bytes.Buffer) (http.Handler, *health.Handler) {
	t.Helper()

	cfg := &config.Config{
		RateLimit: config.RateLimitConfig{RequestsPerMinute: 3},
	}
	logger := slog.New(slog.NewJSONHandler(logs, nil))

	// Requests in these tests never reach the database, so a nil pool is enough.
	probes := health.NewHandler(nil)
	// Stub route: modules has no domains yet, and chi skips middleware on an empty subrouter.
	ping := func(r chi.Router) {
		r.Get("/ping", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	}

	return newRouter(cfg, logger, probes, ping), probes
}

func serve(h http.Handler, method, path string, header http.Header) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(`{}`))
	req.RemoteAddr = "203.0.113.10:1234"
	for k, v := range header {
		req.Header[k] = v
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestRouterErrorsAreJSON(t *testing.T) {
	h, _ := testRouter(t, &bytes.Buffer{})

	tests := []struct {
		method, path string
		status       int
		code         string
	}{
		{http.MethodGet, "/nope", http.StatusNotFound, "NOT_FOUND"},
		{http.MethodGet, "/api/v1/nope", http.StatusNotFound, "NOT_FOUND"},
		{http.MethodPost, "/health", http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED"},
	}

	for _, tt := range tests {
		rec := serve(h, tt.method, tt.path, nil)

		var body struct {
			Error struct{ Code string } `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Errorf("%s %s: non-JSON body %q", tt.method, tt.path, rec.Body)
			continue
		}
		if rec.Code != tt.status || body.Error.Code != tt.code {
			t.Errorf("%s %s: got %d %s, want %d %s", tt.method, tt.path, rec.Code, body.Error.Code, tt.status, tt.code)
		}
	}
}

func TestRouterSecurityAndRequestIDHeaders(t *testing.T) {
	h, _ := testRouter(t, &bytes.Buffer{})

	rec := serve(h, http.MethodGet, "/health", http.Header{"X-Request-Id": {"bad id with spaces"}})

	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q", got)
	}
	if got := rec.Header().Get("X-Request-ID"); got == "" || strings.Contains(got, " ") {
		t.Errorf("invalid client request ID should be replaced, got %q", got)
	}
}

func TestRouterRateLimitsAPIButNotProbes(t *testing.T) {
	h, _ := testRouter(t, &bytes.Buffer{})

	for range 3 {
		serve(h, http.MethodGet, "/api/v1/ping", nil)
	}
	if rec := serve(h, http.MethodGet, "/api/v1/ping", nil); rec.Code != http.StatusTooManyRequests {
		t.Errorf("4th API request: %d", rec.Code)
	}
	for range 5 {
		if rec := serve(h, http.MethodGet, "/health", nil); rec.Code != http.StatusOK {
			t.Fatalf("probe rate limited: %d", rec.Code)
		}
	}
}

func TestRouterProbeLogging(t *testing.T) {
	var logs bytes.Buffer
	h, probes := testRouter(t, &logs)

	serve(h, http.MethodGet, "/health", nil)
	if logs.Len() != 0 {
		t.Fatalf("successful probe should not be logged: %s", logs.String())
	}

	probes.StartDraining()
	serve(h, http.MethodGet, "/ready", nil)

	if !strings.Contains(logs.String(), `"level":"WARN"`) || !strings.Contains(logs.String(), `"status":503`) {
		t.Errorf("failed probe should be logged at WARN: %s", logs.String())
	}
}
