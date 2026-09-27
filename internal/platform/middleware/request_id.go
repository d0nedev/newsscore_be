package middleware

import (
	"github.com/d0nedev/newsscore/internal/platform/requestcontext"
	"net/http"
	"regexp"
	"uuid"
)

const RequestIDHeader = "X-Request-ID"

// Client-supplied IDs end up in logs and response headers; anything else is replaced.
var validRequestID = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)

func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get(RequestIDHeader)

		if !validRequestID.MatchString(requestID) {
			requestID = uuid.New().String()
		}

		w.Header().Set(RequestIDHeader, requestID)

		next.ServeHTTP(w, r.WithContext(requestcontext.WithRequestID(r.Context(), requestID)))
	})
}
