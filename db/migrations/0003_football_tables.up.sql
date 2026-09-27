CREATE TABLE teams (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flashscore_id TEXT NOT NULL UNIQUE,
    name          TEXT NOT NULL,
    short_name    TEXT NOT NULL,
    logo_url      TEXT,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE matches (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flashscore_id TEXT NOT NULL UNIQUE,
    season        SMALLINT NOT NULL, -- ponytail: season year until seasons table exists
    home_team_id  UUID NOT NULL REFERENCES teams(id),
    away_team_id  UUID NOT NULL REFERENCES teams(id),
    status        TEXT NOT NULL,
    match_time    TIMESTAMPTZ NOT NULL,
    home_score    SMALLINT,
    away_score    SMALLINT,
    data_as_of    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE standings (
    season     SMALLINT NOT NULL,
    team_id    UUID NOT NULL REFERENCES teams(id),
    rank       SMALLINT NOT NULL,
    points     SMALLINT NOT NULL,
    form       TEXT,
    zone       TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (season, team_id)
);
