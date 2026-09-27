package middleware

import "net/http"

// SecurityHeaders sets headers appropriate for a JSON-only API. HSTS is left to
// the TLS terminator; CORS lives in cors.go.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		h.Set("Cache-Control", "no-store")

		next.ServeHTTP(w, r)
	})
}
