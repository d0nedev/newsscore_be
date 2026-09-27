-- Standings are computed from finished matches at read time (db/queries/standings.sql):
-- always consistent with results, and cheap at league size (18 teams, ~300 matches).
DROP TABLE standings;
