package middleware

import (
	"crypto/sha256"
	"crypto/subtle"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"net/http"
)

const APIKeyHeader = "X-API-Key"

// APIKey rejects requests without a valid X-API-Key. With no keys configured it
// is a no-op; config validation only allows that in development.
func APIKey(keys []string) func(http.Handler) http.Handler {
	hashes := make([][32]byte, 0, len(keys))
	for _, key := range keys {
		hashes = append(hashes, sha256.Sum256([]byte(key)))
	}

	return func(next http.Handler) http.Handler {
		if len(hashes) == 0 {
			return next
		}

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := sha256.Sum256([]byte(r.Header.Get(APIKeyHeader)))

			valid := 0
			for _, want := range hashes {
				valid |= subtle.ConstantTimeCompare(got[:], want[:])
			}

			if valid != 1 {
				httpx.WriteError(w, apperror.New(
					http.StatusUnauthorized,
					apperror.CodeUnauthorized,
					"missing or invalid API key",
				))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
