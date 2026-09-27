package middleware

import (
	"errors"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/httpx"
	"net"
	"net/http"
	"net/netip"
	"time"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/httprate"
)

var errClientIPUnresolved = errors.New("client IP could not be determined")

// ClientIP resolves the client IP into the request context. X-Forwarded-For is
// honored only when the TCP peer itself is a trusted proxy: chi's
// ClientIPFromXFF reads the header alone, so a client reaching the server
// directly could otherwise forge its IP.
func ClientIP(trustedProxies []string) func(http.Handler) http.Handler {
	if len(trustedProxies) == 0 {
		return chimiddleware.ClientIPFromRemoteAddr
	}

	prefixes := make([]netip.Prefix, 0, len(trustedProxies))
	for _, p := range trustedProxies {
		prefixes = append(prefixes, netip.MustParsePrefix(p))
	}

	fromXFF := chimiddleware.ClientIPFromXFF(trustedProxies...)

	return func(next http.Handler) http.Handler {
		viaProxy := fromXFF(next)
		direct := chimiddleware.ClientIPFromRemoteAddr(next)

		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if peerIsTrusted(r.RemoteAddr, prefixes) {
				viaProxy.ServeHTTP(w, r)
				return
			}

			direct.ServeHTTP(w, r)
		})
	}
}

func peerIsTrusted(remoteAddr string, prefixes []netip.Prefix) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}

	addr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}

	addr = addr.Unmap()
	for _, p := range prefixes {
		if p.Contains(addr) {
			return true
		}
	}

	return false
}

// RateLimit limits requests per client IP. It must run after ClientIP.
func RateLimit(requestsPerMinute int) func(http.Handler) http.Handler {
	return httprate.LimitBy(
		requestsPerMinute,
		time.Minute,
		clientIPKey,
		httprate.WithLimitHandler(func(w http.ResponseWriter, r *http.Request) {
			httpx.WriteError(w, apperror.New(
				http.StatusTooManyRequests,
				apperror.CodeRateLimited,
				"too many requests",
			))
		}),
		httprate.WithErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
			httpx.WriteError(w, apperror.New(
				http.StatusBadRequest,
				apperror.CodeClientIPUnresolved,
				err.Error(),
			))
		}),
	)
}

// clientIPKey fails closed: an unresolved IP would otherwise put every such
// request into one shared bucket.
func clientIPKey(r *http.Request) (string, error) {
	ip := chimiddleware.GetClientIP(r.Context())
	if ip == "" {
		return "", errClientIPUnresolved
	}

	return httprate.CanonicalizeIP(ip), nil
}
