package news

import (
	"context"
	"net/http"
	"time"
	"uuid"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/platform/tracing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/trace"
)

type Service struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	tracer  trace.Tracer
}

func NewService(pool *pgxpool.Pool, queries *db.Queries, tracer trace.Tracer) *Service {
	return &Service{pool: pool, queries: queries, tracer: tracer}
}

// optUUID maps an optional filter to a nullable query argument.
func optUUID(id *uuid.UUID) pgtype.UUID {
	if id == nil {
		return pgtype.UUID{}
	}
	return database.UUID(*id)
}

func (s *Service) List(ctx context.Context, f listFilter) ([]db.ListPublishedNewsRow, *cursor, error) {
	ctx, span := s.tracer.Start(ctx, "NewsService.List")
	defer span.End()

	params := db.ListPublishedNewsParams{
		MatchID:  optUUID(f.MatchID),
		TeamID:   optUUID(f.TeamID),
		PlayerID: optUUID(f.PlayerID),
		RowLimit: int32(f.Limit + 1),
	}
	if f.After != nil {
		params.CursorPublishedAt = pgtype.Timestamptz{Time: f.After.PublishedAt, Valid: true}
		params.CursorID = database.UUID(f.After.ID)
	}

	rows, err := s.queries.ListPublishedNews(ctx, params)
	if err != nil {
		return nil, nil, tracing.Fail(span, apperror.Internal(CodeNewsQueryFailed, "failed to list news", err))
	}
	if len(rows) <= f.Limit {
		return rows, nil, nil
	}

	rows = rows[:f.Limit]
	last := rows[f.Limit-1]
	return rows, &cursor{PublishedAt: last.PublishedAt.Time, ID: last.ID.Bytes}, nil
}

func (s *Service) GetBySlug(ctx context.Context, slug string) (db.GetPublishedNewsBySlugRow, error) {
	ctx, span := s.tracer.Start(ctx, "NewsService.GetBySlug")
	defer span.End()

	row, err := s.queries.GetPublishedNewsBySlug(ctx, slug)
	if err != nil {
		return row, mapError(span, err, CodeNewsQueryFailed, "failed to get news")
	}
	return row, nil
}

func (s *Service) ListAll(ctx context.Context) ([]db.ListAllNewsRow, error) {
	ctx, span := s.tracer.Start(ctx, "NewsService.ListAll")
	defer span.End()

	rows, err := s.queries.ListAllNews(ctx)
	if err != nil {
		return nil, tracing.Fail(span, apperror.Internal(CodeNewsQueryFailed, "failed to list news", err))
	}
	return rows, nil
}

func (s *Service) Get(ctx context.Context, id uuid.UUID) (AdminNews, error) {
	ctx, span := s.tracer.Start(ctx, "NewsService.Get")
	defer span.End()

	row, err := s.queries.GetNews(ctx, database.UUID(id))
	if err != nil {
		return AdminNews{}, mapError(span, err, CodeNewsQueryFailed, "failed to get news")
	}
	rows, err := s.queries.ListNewsLinks(ctx, row.ID)
	if err != nil {
		return AdminNews{}, tracing.Fail(span, apperror.Internal(CodeNewsQueryFailed, "failed to get news links", err))
	}

	links := Links{MatchIDs: []string{}, TeamIDs: []string{}, PlayerIDs: []string{}}
	for _, l := range rows {
		switch {
		case l.MatchID.Valid:
			links.MatchIDs = append(links.MatchIDs, l.MatchID.String())
		case l.TeamID.Valid:
			links.TeamIDs = append(links.TeamIDs, l.TeamID.String())
		case l.PlayerID.Valid:
			links.PlayerIDs = append(links.PlayerIDs, l.PlayerID.String())
		}
	}

	return AdminNews{
		ID: row.ID.String(), Slug: row.Slug, Category: row.Category, Title: row.Title,
		Summary: row.Summary, Body: row.Body, Image: row.ImageUrl.String,
		PublishedAt: timePtr(row.PublishedAt), UpdatedAt: row.UpdatedAt.Time.UTC().Format(time.RFC3339),
		Links: &links,
	}, nil
}

// Create stores an article and its links in one transaction.
func (s *Service) Create(ctx context.Context, v validated, authorID string) (uuid.UUID, error) {
	ctx, span := s.tracer.Start(ctx, "NewsService.Create")
	defer span.End()

	var author pgtype.UUID
	_ = author.Scan(authorID)

	var created pgtype.UUID
	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		var err error
		created, err = q.CreateNews(ctx, db.CreateNewsParams{
			Slug: v.Slug, Category: v.Category, Title: v.Title, Summary: v.Summary, Body: v.Body,
			ImageUrl:    pgtype.Text{String: v.Image, Valid: v.Image != ""},
			AuthorID:    author,
			PublishedAt: pgtype.Timestamptz{Time: time.Now(), Valid: v.Published},
		})
		if err != nil {
			return err
		}
		return addLinks(ctx, q, created, v.links)
	})
	if err != nil {
		return uuid.UUID{}, mapWriteError(span, err)
	}
	return created.Bytes, nil
}

// Update replaces an article's fields and links in one transaction.
func (s *Service) Update(ctx context.Context, id uuid.UUID, v validated) error {
	ctx, span := s.tracer.Start(ctx, "NewsService.Update")
	defer span.End()

	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		updated, err := q.UpdateNews(ctx, db.UpdateNewsParams{
			ID: database.UUID(id), Slug: v.Slug, Category: v.Category, Title: v.Title, Summary: v.Summary, Body: v.Body,
			ImageUrl: pgtype.Text{String: v.Image, Valid: v.Image != ""},
			Publish:  v.Published,
		})
		if err != nil {
			return err
		}
		if err := q.DeleteNewsLinks(ctx, updated); err != nil {
			return err
		}
		return addLinks(ctx, q, updated, v.links)
	})
	if err != nil {
		return mapWriteError(span, err)
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, id uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "NewsService.Delete")
	defer span.End()

	n, err := s.queries.DeleteNews(ctx, database.UUID(id))
	if err != nil {
		return tracing.Fail(span, apperror.Internal(CodeNewsWriteFailed, "failed to delete news", err))
	}
	if n == 0 {
		return apperror.NotFound(CodeNewsNotFound, "news not found")
	}
	return nil
}

func addLinks(ctx context.Context, q *db.Queries, newsID pgtype.UUID, links []link) error {
	for _, l := range links {
		if err := q.AddNewsLink(ctx, db.AddNewsLinkParams{NewsID: newsID, MatchID: l.Match, TeamID: l.Team, PlayerID: l.Player}); err != nil {
			return err
		}
	}
	return nil
}

func mapError(span trace.Span, err error, code, message string) error {
	if database.IsNotFound(err) {
		return apperror.NotFound(CodeNewsNotFound, "news not found")
	}
	return tracing.Fail(span, apperror.Internal(code, message, err))
}

func mapWriteError(span trace.Span, err error) error {
	switch {
	case database.IsNotFound(err):
		return apperror.NotFound(CodeNewsNotFound, "news not found")
	case database.IsUniqueViolation(err):
		return apperror.Wrap(http.StatusConflict, CodeNewsSlugTaken, "slug is already used by another article", err)
	case database.IsForeignKeyViolation(err):
		return apperror.Validation("links reference a match, team, or player that does not exist")
	}
	return tracing.Fail(span, apperror.Internal(CodeNewsWriteFailed, "failed to save news", err))
}
