CREATE TABLE news (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug         TEXT NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9]+(-[a-z0-9]+)*$'),
    category     TEXT NOT NULL,             -- "Resmi", "Wawancara", "Cedera", "Statistik"
    title        TEXT NOT NULL CHECK (length(btrim(title)) > 0),
    summary      TEXT NOT NULL,
    body         TEXT NOT NULL DEFAULT '',
    image_url    TEXT,
    author_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    published_at TIMESTAMPTZ,               -- null = draft
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX news_published_idx ON news (published_at DESC, id DESC) WHERE published_at IS NOT NULL;

-- News attach to matches, teams, and players: the product's differentiator.
-- One row per link, exactly one target, so each target keeps a real foreign key.
CREATE TABLE news_links (
    news_id   UUID NOT NULL REFERENCES news(id) ON DELETE CASCADE,
    match_id  UUID REFERENCES matches(id) ON DELETE CASCADE,
    team_id   UUID REFERENCES teams(id) ON DELETE CASCADE,
    player_id UUID REFERENCES players(id) ON DELETE CASCADE,
    CHECK (num_nonnulls(match_id, team_id, player_id) = 1)
);
CREATE INDEX news_links_news_idx   ON news_links (news_id);
CREATE INDEX news_links_match_idx  ON news_links (match_id)  WHERE match_id IS NOT NULL;
CREATE INDEX news_links_team_idx   ON news_links (team_id)   WHERE team_id IS NOT NULL;
CREATE INDEX news_links_player_idx ON news_links (player_id) WHERE player_id IS NOT NULL;
