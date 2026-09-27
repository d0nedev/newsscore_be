package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

var ok = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
})

func TestAPIKey(t *testing.T) {
	h := APIKey([]string{"old-key", "new-key"})(ok)

	tests := []struct {
		name string
		key  string
		want int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "nope", http.StatusUnauthorized},
		{"prefix of valid", "new", http.StatusUnauthorized},
		{"first key", "old-key", http.StatusNoContent},
		{"rotated key", "new-key", http.StatusNoContent},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/", nil)
			if tt.key != "" {
				req.Header.Set(APIKeyHeader, tt.key)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestAPIKeyDisabledWithoutKeys(t *testing.T) {
	rec := httptest.NewRecorder()
	APIKey(nil)(ok).ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d", rec.Code)
	}
}

func TestRouteTagNamesSpanWithPattern(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))

	r := chi.NewRouter()
	r.Use(RouteTag)
	r.Get("/matches/{id}", ok)

	h := otelhttp.NewHandler(r, "svc", otelhttp.WithTracerProvider(tp))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/matches/123", nil))

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d", len(spans))
	}
	if got := spans[0].Name(); got != "GET /matches/{id}" {
		t.Errorf("span name = %q", got)
	}
}

func TestPanicIsAccessLoggedAs500(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	panicking := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") })

	// Same order as app.New: Logging outside Recovery.
	h := Logging(logger)(Recovery(logger)(panicking))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil).WithContext(context.Background()))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}

	found := false
	for _, line := range bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n")) {
		var entry map[string]any
		if err := json.Unmarshal(line, &entry); err != nil {
			t.Fatal(err)
		}
		if entry["msg"] == "http request completed" && entry["status"] == float64(500) {
			found = true
		}
	}
	if !found {
		t.Errorf("no access log with status 500:\n%s", buf.String())
	}
}

func TestRequestIDValidation(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = w.Header().Get(RequestIDHeader)
	}))

	tests := []struct {
		in       string
		keepsOwn bool
	}{
		{"abc-123_DEF.4", true},
		{"", false},
		{strings.Repeat("a", 65), false},
		{"id\nforged-log-line", false},
		{"<script>", false},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set(RequestIDHeader, tt.in)
		h.ServeHTTP(httptest.NewRecorder(), req)

		if (seen == tt.in) != tt.keepsOwn || seen == "" {
			t.Errorf("in %q: got %q, keepsOwn %v", tt.in, seen, tt.keepsOwn)
		}
	}
}

func TestRecoveryAfterHeadersSentDoesNotRewrite(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))

	h := Recovery(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		panic("boom")
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK || rec.Body.String() != "partial" {
		t.Errorf("response was rewritten: %d %q", rec.Code, rec.Body)
	}
}

func TestRecoveryRepanicsErrAbortHandler(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	h := Recovery(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(http.ErrAbortHandler)
	}))

	defer func() {
		if recover() != http.ErrAbortHandler {
			t.Error("ErrAbortHandler must propagate to net/http")
		}
	}()

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestResponseWriterSupportsResponseController(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec}

	if err := http.NewResponseController(rw).Flush(); err != nil {
		t.Errorf("Flush through wrapper: %v", err)
	}
}

func TestLoggingIncludesBodyOnFailure(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))

	h := Logging(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.WriteHeader(http.StatusBadRequest)
	}))

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":1}`)))

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["request_body"] != `{"name":1}` {
		t.Errorf("request_body = %v", entry["request_body"])
	}
}

func TestRequestTimeoutCancelsContext(t *testing.T) {
	h := RequestTimeout(10 * time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			w.WriteHeader(http.StatusGatewayTimeout)
		case <-time.After(time.Second):
			w.WriteHeader(http.StatusOK)
		}
	}))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusGatewayTimeout {
		t.Errorf("status = %d, want context cancelled before handler finished", rec.Code)
	}
}
