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
