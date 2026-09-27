-- name: SearchTeams :many
SELECT id, name, short_name, logo_url
FROM teams
WHERE sqlc.arg(q)::text <% name
ORDER BY word_similarity(sqlc.arg(q)::text, name) DESC, name
LIMIT 5;

-- name: SearchPlayers :many
SELECT p.id, p.name, p.shirt_number, t.name AS team_name
FROM players p
LEFT JOIN teams t ON t.id = p.team_id
WHERE sqlc.arg(q)::text <% p.name
ORDER BY word_similarity(sqlc.arg(q)::text, p.name) DESC, p.name
LIMIT 10;

-- name: SearchNews :many
SELECT slug, title, category, published_at
FROM news
WHERE published_at IS NOT NULL AND published_at <= now()
  AND sqlc.arg(q)::text <% title
ORDER BY word_similarity(sqlc.arg(q)::text, title) DESC, published_at DESC
LIMIT 5;
