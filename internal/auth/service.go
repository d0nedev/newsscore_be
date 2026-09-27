package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/netip"
	"time"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/tracing"

	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/trace"
)

// User is the signed-in account attached to a request.
type User struct {
	ID    string
	Email string
	Name  string
	Role  string
}

type Service struct {
	queries *db.Queries
	tracer  trace.Tracer
	ttl     time.Duration
	// dummyHash is verified when the email is unknown, so a miss costs as much
	// as a wrong password and response time does not reveal which emails exist.
	dummyHash string
}

func NewService(queries *db.Queries, tracer trace.Tracer, ttl time.Duration) *Service {
	dummy, err := HashPassword("not-a-real-password")
	if err != nil {
		panic(err) // crypto/rand failing at startup is not recoverable
	}
	return &Service{queries: queries, tracer: tracer, ttl: ttl, dummyHash: dummy}
}

type session struct {
	Token     string
	ExpiresAt time.Time
}

// Login checks credentials and opens a session. The returned token goes in the cookie only.
func (s *Service) Login(ctx context.Context, email, password, userAgent string, ip netip.Addr) (User, session, error) {
	ctx, span := s.tracer.Start(ctx, "AuthService.Login")
	defer span.End()

	invalid := apperror.New(http.StatusUnauthorized, CodeInvalidCredentials, "invalid email or password")

	row, err := s.queries.GetUserByEmail(ctx, email)
	if err != nil {
		if !database.IsNotFound(err) {
			return User{}, session{}, tracing.Fail(span, apperror.Internal(CodeAuthFailed, "failed to sign in", err))
		}
		_, _ = VerifyPassword(password, s.dummyHash)
		return User{}, session{}, invalid
	}

	ok, err := VerifyPassword(password, row.PasswordHash)
	if err != nil {
		return User{}, session{}, tracing.Fail(span, apperror.Internal(CodeAuthFailed, "failed to sign in", err))
	}
	if !ok {
		return User{}, session{}, invalid
	}

	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return User{}, session{}, tracing.Fail(span, apperror.Internal(CodeAuthFailed, "failed to sign in", err))
	}
	sess := session{Token: base64.RawURLEncoding.EncodeToString(raw), ExpiresAt: time.Now().Add(s.ttl)}

	params := db.CreateSessionParams{
		TokenHash: tokenHash(sess.Token),
		UserID:    row.ID,
		ExpiresAt: pgtype.Timestamptz{Time: sess.ExpiresAt, Valid: true},
		UserAgent: pgtype.Text{String: truncate(userAgent, 512), Valid: userAgent != ""},
	}
	if ip.IsValid() {
		params.Ip = &ip
	}
	if err := s.queries.CreateSession(ctx, params); err != nil {
		return User{}, session{}, tracing.Fail(span, apperror.Internal(CodeAuthFailed, "failed to sign in", err))
	}

	return User{ID: row.ID.String(), Email: row.Email, Name: row.Name, Role: row.Role}, sess, nil
}

// UserFromToken resolves a cookie token; ok is false for unknown or expired sessions.
func (s *Service) UserFromToken(ctx context.Context, token string) (User, bool, error) {
	row, err := s.queries.GetSessionUser(ctx, tokenHash(token))
	if err != nil {
		if database.IsNotFound(err) {
			return User{}, false, nil
		}
		return User{}, false, apperror.Internal(CodeAuthFailed, "failed to check session", err)
	}
	return User{ID: row.ID.String(), Email: row.Email, Name: row.Name, Role: row.Role}, true, nil
}

func (s *Service) Logout(ctx context.Context, token string) error {
	if err := s.queries.DeleteSession(ctx, tokenHash(token)); err != nil {
		return apperror.Internal(CodeAuthFailed, "failed to sign out", err)
	}
	return nil
}

func tokenHash(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
