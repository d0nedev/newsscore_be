-- name: LatestSeason :one
SELECT coalesce(max(season), 0)::smallint FROM matches;

-- name: ListCompetitions :many
-- Each competition with its latest season (0 until the ingestor has stored a match).
SELECT c.id, c.slug, c.name, c.country, c.type,
       coalesce((SELECT max(season) FROM matches m WHERE m.competition_id = c.id), 0)::smallint AS season
FROM competitions c
WHERE c.active
ORDER BY c.sort_order;

-- name: GetCompetition :one
SELECT c.id, c.slug, c.name, c.country, c.type,
       coalesce((SELECT max(season) FROM matches m WHERE m.competition_id = c.id), 0)::smallint AS season
FROM competitions c
WHERE c.slug = $1;

-- name: ListStandings :many
-- Every team with a match in the season, ranked by points, goal difference, goals scored.
-- ponytail: Liga 1 breaks ties head-to-head first; add that if two teams ever tie on points at season end.
WITH season_teams AS (
    SELECT home_team_id AS team_id FROM matches WHERE competition_id = sqlc.arg(competition_id)::uuid AND season = sqlc.arg(season)::smallint
    UNION
    SELECT away_team_id FROM matches WHERE competition_id = sqlc.arg(competition_id)::uuid AND season = sqlc.arg(season)::smallint
),
results AS (
    SELECT home_team_id AS team_id, home_score AS gf, away_score AS ga, match_time
    FROM matches WHERE competition_id = sqlc.arg(competition_id)::uuid AND season = sqlc.arg(season)::smallint AND status = 'finished'
    UNION ALL
    SELECT away_team_id, away_score, home_score, match_time
    FROM matches WHERE competition_id = sqlc.arg(competition_id)::uuid AND season = sqlc.arg(season)::smallint AND status = 'finished'
),
totals AS (
    SELECT st.team_id,
           count(r.team_id)::int                       AS played,
           count(*) FILTER (WHERE r.gf > r.ga)::int    AS won,
           count(*) FILTER (WHERE r.gf = r.ga)::int    AS drawn,
           count(*) FILTER (WHERE r.gf < r.ga)::int    AS lost,
           coalesce(sum(r.gf), 0)::int                 AS goals_for,
           coalesce(sum(r.ga), 0)::int                 AS goals_against,
           coalesce((array_agg(CASE WHEN r.gf > r.ga THEN 'W' WHEN r.gf = r.ga THEN 'D' ELSE 'L' END
                                ORDER BY r.match_time DESC) FILTER (WHERE r.team_id IS NOT NULL))[1:5],
                    '{}')::text[]                     AS form
    FROM season_teams st
    LEFT JOIN results r ON r.team_id = st.team_id
    GROUP BY st.team_id
)
SELECT t.id, t.name, t.short_name, t.logo_url,
       tt.played, tt.won, tt.drawn, tt.lost, tt.goals_for, tt.goals_against,
       (tt.won * 3 + tt.drawn)::int AS points,
       tt.form
FROM totals tt
JOIN teams t ON t.id = tt.team_id
ORDER BY points DESC, tt.goals_for - tt.goals_against DESC, tt.goals_for DESC, t.name;
