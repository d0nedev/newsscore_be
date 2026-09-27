-- name: ListPublishedNews :many
-- Keyset pagination on (published_at, id), newest first.
SELECT n.id, n.slug, n.category, n.title, n.summary, n.image_url, n.published_at
FROM news n
WHERE n.published_at IS NOT NULL AND n.published_at <= now()
  AND (sqlc.narg(match_id)::uuid IS NULL OR EXISTS (SELECT 1 FROM news_links l WHERE l.news_id = n.id AND l.match_id = sqlc.narg(match_id)::uuid))
  AND (sqlc.narg(team_id)::uuid IS NULL OR EXISTS (SELECT 1 FROM news_links l WHERE l.news_id = n.id AND l.team_id = sqlc.narg(team_id)::uuid))
  AND (sqlc.narg(player_id)::uuid IS NULL OR EXISTS (SELECT 1 FROM news_links l WHERE l.news_id = n.id AND l.player_id = sqlc.narg(player_id)::uuid))
  AND (sqlc.narg(cursor_published_at)::timestamptz IS NULL
       OR (n.published_at, n.id) < (sqlc.narg(cursor_published_at)::timestamptz, sqlc.narg(cursor_id)::uuid))
ORDER BY n.published_at DESC, n.id DESC
LIMIT sqlc.arg(row_limit);

-- name: GetPublishedNewsBySlug :one
SELECT id, slug, category, title, summary, body, image_url, published_at
FROM news
WHERE slug = $1 AND published_at IS NOT NULL AND published_at <= now();

-- name: ListAllNews :many
-- Editor view: drafts included, newest edits first.
SELECT id, slug, category, title, published_at, updated_at
FROM news
ORDER BY updated_at DESC
LIMIT 100;

-- name: GetNews :one
SELECT id, slug, category, title, summary, body, image_url, published_at, created_at, updated_at
FROM news
WHERE id = $1;

-- name: CreateNews :one
INSERT INTO news (slug, category, title, summary, body, image_url, author_id, published_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
RETURNING id;

-- name: UpdateNews :one
-- published_at keeps its first value on re-save, so editing a live article does not bump it to the top.
UPDATE news
SET slug = $2, category = $3, title = $4, summary = $5, body = $6, image_url = $7,
    published_at = CASE WHEN sqlc.arg(publish)::boolean THEN coalesce(published_at, now()) ELSE NULL END,
    updated_at = now()
WHERE id = $1
RETURNING id;

-- name: DeleteNews :execrows
DELETE FROM news WHERE id = $1;

-- name: DeleteNewsLinks :exec
DELETE FROM news_links WHERE news_id = $1;

-- name: AddNewsLink :exec
INSERT INTO news_links (news_id, match_id, team_id, player_id)
VALUES ($1, $2, $3, $4);

-- name: ListNewsLinks :many
SELECT match_id, team_id, player_id FROM news_links WHERE news_id = $1;
