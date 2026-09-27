CREATE TABLE match_statistics (
    match_id   UUID PRIMARY KEY REFERENCES matches(id) ON DELETE CASCADE,
    stats      JSONB NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE match_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    match_id      UUID NOT NULL REFERENCES matches(id) ON DELETE CASCADE,
    flashscore_id TEXT NOT NULL,
    type          TEXT NOT NULL,
    minute        TEXT NOT NULL,
    player_name   TEXT NOT NULL,
    team_id       UUID REFERENCES teams(id),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (match_id, flashscore_id)
);

CREATE TABLE players (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    flashscore_id TEXT NOT NULL UNIQUE,
    team_id       UUID REFERENCES teams(id),
    name          TEXT NOT NULL,
    avatar_url    TEXT,
    position      TEXT,
    nationality   TEXT,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
