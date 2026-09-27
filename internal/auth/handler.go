package auth

import (
	"context"
	"net/http"
	"net/netip"
	"strings"
	"time"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/httpx"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

const CookieName = "session"

type Handler struct {
	service *Service
	secure  bool
}

func NewHandler(service *Service, secureCookie bool) *Handler {
	return &Handler{service: service, secure: secureCookie}
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type UserResponse struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type LoginResponse struct {
	Data struct {
		User UserResponse `json:"user"`
	} `json:"data"`
}

type MeResponse struct {
	Data UserResponse `json:"data"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) error {
	var req LoginRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	req.Email = strings.TrimSpace(req.Email)
	switch {
	case req.Email == "" || !strings.Contains(req.Email, "@"):
		return apperror.Validation("email is required")
	case req.Password == "":
		return apperror.Validation("password is required")
	case len(req.Password) > 1024: // argon2 cost is per byte; cap it
		return apperror.Validation("password is too long")
	}

	ip, _ := netip.ParseAddr(chimiddleware.GetClientIP(r.Context()))
	user, sess, err := h.service.Login(r.Context(), req.Email, req.Password, r.UserAgent(), ip)
	if err != nil {
		return err
	}

	http.SetCookie(w, h.cookie(sess.Token, sess.ExpiresAt))

	var resp LoginResponse
	resp.Data.User = toUserResponse(user)
	return httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(CookieName); err == nil && c.Value != "" {
		if err := h.service.Logout(r.Context(), c.Value); err != nil {
			return err
		}
	}

	http.SetCookie(w, h.cookie("", time.Unix(0, 0)))
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) error {
	user, _ := UserFrom(r.Context())
	return httpx.WriteJSON(w, http.StatusOK, MeResponse{Data: toUserResponse(user)})
}

// cookie builds the session cookie. SameSite=Lax keeps it off cross-site POSTs (CSRF)
// while still sending it to an API on a sibling subdomain of the frontend.
func (h *Handler) cookie(value string, expires time.Time) *http.Cookie {
	c := &http.Cookie{
		Name:     CookieName,
		Value:    value,
		Path:     "/",
		Expires:  expires,
		HttpOnly: true,
		Secure:   h.secure,
		SameSite: http.SameSiteLaxMode,
	}
	if value == "" {
		c.MaxAge = -1
	}
	return c
}

func toUserResponse(u User) UserResponse {
	return UserResponse{ID: u.ID, Email: u.Email, Name: u.Name}
}

type userKey struct{}

// UserFrom returns the signed-in user set by RequireUser.
func UserFrom(ctx context.Context) (User, bool) {
	u, ok := ctx.Value(userKey{}).(User)
	return u, ok
}

// RequireUser rejects requests without a valid session cookie.
func (h *Handler) RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(CookieName)
		if err != nil || c.Value == "" {
			httpx.WriteError(w, apperror.New(http.StatusUnauthorized, apperror.CodeUnauthorized, "sign in required"))
			return
		}

		user, ok, err := h.service.UserFromToken(r.Context(), c.Value)
		if err != nil {
			httpx.WriteError(w, err)
			return
		}
		if !ok {
			http.SetCookie(w, h.cookie("", time.Unix(0, 0)))
			httpx.WriteError(w, apperror.New(http.StatusUnauthorized, apperror.CodeUnauthorized, "session expired"))
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	})
}
