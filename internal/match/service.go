package match

import (
	"context"
	"uuid"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/tracing"

	"github.com/jackc/pgx/v5/pgtype"
	"go.opentelemetry.io/otel/trace"
)

type Service struct {
	queries *db.Queries
	tracer  trace.Tracer
}

func NewService(queries *db.Queries, tracer trace.Tracer) *Service {
	return &Service{queries: queries, tracer: tracer}
}

type matchDetail struct {
	Match   db.MatchRow
	Events  []db.ListMatchEventsRow
	Lineups []db.ListMatchLineupsRow
	Stats   []byte // JSONB array of {label, home, away}; nil when not scraped yet
}

func (s *Service) List(ctx context.Context, f listFilter) ([]db.MatchRow, error) {
	ctx, span := s.tracer.Start(ctx, "MatchService.List")
	defer span.End()

	params := db.ListMatchesParams{
		FromTime:    pgtype.Timestamptz{Time: f.From, Valid: true},
		ToTime:      pgtype.Timestamptz{Time: f.From.AddDate(0, 0, 1), Valid: true},
		Status:      pgtype.Text{String: f.Status, Valid: f.Status != ""},
		Competition: pgtype.Text{String: f.League, Valid: f.League != ""},
	}
	if f.TeamID != nil {
		params.TeamID = database.UUID(*f.TeamID)
	}

	matches, err := s.queries.ListMatches(ctx, params)
	if err != nil {
		return nil, tracing.Fail(span, apperror.Internal(CodeMatchQueryFailed, "failed to list matches", err))
	}

	return matches, nil
}

func (s *Service) FindByID(ctx context.Context, id uuid.UUID) (matchDetail, error) {
	ctx, span := s.tracer.Start(ctx, "MatchService.FindByID")
	defer span.End()

	row, err := s.queries.GetMatch(ctx, database.UUID(id))
	if err != nil {
		if database.IsNotFound(err) {
			return matchDetail{}, apperror.NotFound(CodeMatchNotFound, "match not found")
		}
		return matchDetail{}, tracing.Fail(span, apperror.Internal(CodeMatchQueryFailed, "failed to get match", err))
	}

	events, err := s.queries.ListMatchEvents(ctx, row.ID)
	if err != nil {
		return matchDetail{}, tracing.Fail(span, apperror.Internal(CodeMatchQueryFailed, "failed to list match events", err))
	}

	stats, err := s.queries.GetMatchStats(ctx, row.ID)
	if err != nil && !database.IsNotFound(err) {
		return matchDetail{}, tracing.Fail(span, apperror.Internal(CodeMatchQueryFailed, "failed to get match stats", err))
	}

	lineups, err := s.queries.ListMatchLineups(ctx, row.ID)
	if err != nil {
		return matchDetail{}, tracing.Fail(span, apperror.Internal(CodeMatchQueryFailed, "failed to list lineups", err))
	}

	return matchDetail{Match: row, Events: events, Lineups: lineups, Stats: stats}, nil
}
