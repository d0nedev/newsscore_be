package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/d0nedev/newsscore/internal/domain"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/provider/flashscore"

	"github.com/jackc/pgx/v5/pgtype"
)

type Worker struct {
	db            *db.Queries
	logger        *slog.Logger
	detailsPerRun int
	assetsDir     string
}

func NewWorker(queries *db.Queries, logger *slog.Logger, detailsPerRun int, assetsDir string) *Worker {
	return &Worker{db: queries, logger: logger, detailsPerRun: detailsPerRun, assetsDir: assetsDir}
}

// Sync pulls the league page (fixtures and results), then fills in details for finished matches that lack them.
func (w *Worker) Sync(ctx context.Context) error {
	// Housekeeping rides along: the ingestor is the one process guaranteed to run alone.
	if n, err := w.db.DeleteExpiredSessions(ctx); err != nil {
		w.logger.Error("delete expired sessions failed", slog.Any("error", err))
	} else if n > 0 {
		w.logger.Info("expired sessions deleted", slog.Int64("count", n))
	}

	competitions, err := w.db.ListActiveCompetitions(ctx)
	if err != nil {
		return fmt.Errorf("list competitions: %w", err)
	}
	// One failing page (layout change, block) must not stop the other competitions.
	for _, c := range competitions {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.syncCompetition(ctx, c); err != nil {
			w.logger.Error("sync competition failed", slog.String("competition", c.Slug), slog.Any("error", err))
		}
		time.Sleep(time.Second) // same politeness gap as the feed requests
	}
	if err := w.syncNationalTeams(ctx); err != nil {
		w.logger.Error("sync national teams failed", slog.Any("error", err))
	}
	if err := w.syncImages(ctx); err != nil {
		w.logger.Error("sync images failed", slog.Any("error", err))
	}
	return w.syncDetails(ctx)
}

// photosPerRun caps player photo downloads per league sync (one per second);
// the first fill of ~500 photos then spreads over a few runs.
const photosPerRun = 100

// mirrorItem is one image to copy into assetsDir: its owner, source, and where to record the local path.
type mirrorItem struct {
	id     pgtype.UUID
	name   string // file name without extension, the Flashscore id
	source string
}

// syncImages stores team logos and player photos under assetsDir once, so
// clients load them from us rather than hotlinking Flashscore.
func (w *Worker) syncImages(ctx context.Context) error {
	teams, err := w.db.ListTeamsMissingLogo(ctx)
	if err != nil {
		return fmt.Errorf("list teams missing logo: %w", err)
	}
	logos := make([]mirrorItem, 0, len(teams))
	for _, t := range teams {
		logos = append(logos, mirrorItem{t.ID, t.FlashscoreID, t.LogoSourceUrl})
	}
	if err := w.mirror(ctx, "teams", logos, func(id pgtype.UUID, url string) error {
		return w.db.SetTeamLogo(ctx, db.SetTeamLogoParams{ID: id, LogoUrl: optText(url)})
	}); err != nil {
		return err
	}

	players, err := w.db.ListPlayersMissingPhoto(ctx, photosPerRun)
	if err != nil {
		return fmt.Errorf("list players missing photo: %w", err)
	}
	photos := make([]mirrorItem, 0, len(players))
	for _, p := range players {
		photos = append(photos, mirrorItem{p.ID, p.FlashscoreID, p.PhotoSourceUrl})
	}
	return w.mirror(ctx, "players", photos, func(id pgtype.UUID, url string) error {
		return w.db.SetPlayerPhoto(ctx, db.SetPlayerPhotoParams{ID: id, PhotoUrl: optText(url)})
	})
}

func (w *Worker) mirror(ctx context.Context, kind string, items []mirrorItem, record func(pgtype.UUID, string) error) error {
	if len(items) == 0 {
		return nil
	}
	dir := filepath.Join(w.assetsDir, kind)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	for _, it := range items {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		time.Sleep(time.Second)

		body, ext, err := flashscore.DownloadImage(it.source)
		if err != nil {
			w.logger.Warn("download image failed", slog.String("kind", kind), slog.String("id", it.name), slog.Any("error", err))
			continue
		}
		file := it.name + ext
		if err := writeFileAtomic(filepath.Join(dir, file), body); err != nil {
			return err
		}
		if err := record(it.id, "/assets/"+kind+"/"+file); err != nil {
			return err
		}
	}
	return nil
}

// writeFileAtomic writes via a temp file and rename, so the API never serves half a file.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (w *Worker) syncCompetition(ctx context.Context, c db.ListActiveCompetitionsRow) error {
	matches, teams, err := flashscore.ScrapeLeague(c.FlashscorePath)
	if err != nil {
		return fmt.Errorf("scrape %s: %w", c.FlashscorePath, err)
	}

	w.logger.Info("competition scraped", slog.String("competition", c.Slug), slog.Int("teams", len(teams)), slog.Int("matches", len(matches)))

	teamIDs := w.upsertTeams(ctx, teams, false)
	for _, m := range matches {
		w.upsertMatch(ctx, m, c.ID, teamIDs)
	}
	return nil
}

// syncNationalTeams reads each national team page (Indonesia): its matches in
// every competition it plays, friendlies included. Only these matches are
// stored, not the rest of those competitions.
func (w *Worker) syncNationalTeams(ctx context.Context) error {
	nationals, err := w.db.ListNationalTeamsToScrape(ctx)
	if err != nil {
		return fmt.Errorf("list national teams: %w", err)
	}

	for _, n := range nationals {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		time.Sleep(time.Second)

		path := "/team/" + n.CountrySlug + "/" + n.FlashscoreID + "/results/"
		matches, teams, err := flashscore.ScrapeTeam(path)
		if err != nil {
			w.logger.Error("scrape national team failed", slog.String("path", path), slog.Any("error", err))
			continue
		}
		w.logger.Info("national team scraped", slog.String("team", n.CountrySlug), slog.Int("matches", len(matches)))

		// Every team on a national team's page is a national team too.
		teamIDs := w.upsertTeams(ctx, teams, true)
		competitions := make(map[string]pgtype.UUID)
		for _, m := range matches {
			id, ok := competitions[m.Competition.Path]
			if !ok {
				if id, err = w.upsertTeamPageCompetition(ctx, m.Competition); err != nil {
					w.logger.Error("upsert competition failed", slog.String("path", m.Competition.Path), slog.Any("error", err))
					continue
				}
				competitions[m.Competition.Path] = id
			}
			w.upsertMatch(ctx, m, id, teamIDs)
		}
	}
	return nil
}

func (w *Worker) upsertTeamPageCompetition(ctx context.Context, c domain.Competition) (pgtype.UUID, error) {
	region, err := w.db.UpsertCountry(ctx, db.UpsertCountryParams{Slug: slugify(c.Region), Name: c.Region})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return w.db.UpsertTeamPageCompetition(ctx, db.UpsertTeamPageCompetitionParams{
		Slug:           path.Base(c.Path), // "/football/asia/asean-championship/" -> "asean-championship"
		Name:           c.Name,
		CountryID:      region,
		FlashscorePath: c.Path,
	})
}

// upsertTeams stores teams and returns their ids by Flashscore id. national
// marks each as its country's national team, creating the country if needed.
func (w *Worker) upsertTeams(ctx context.Context, teams []domain.Team, national bool) map[string]pgtype.UUID {
	ids := make(map[string]pgtype.UUID, len(teams))
	for _, t := range teams {
		var country pgtype.UUID
		if national {
			var err error
			if country, err = w.db.UpsertCountry(ctx, db.UpsertCountryParams{Slug: t.Slug, Name: t.Name}); err != nil {
				w.logger.Error("upsert country failed", slog.String("team", t.Name), slog.Any("error", err))
				continue
			}
		}
		id, err := w.db.UpsertTeam(ctx, db.UpsertTeamParams{
			FlashscoreID:  t.FlashscoreID,
			Name:          t.Name,
			ShortName:     t.ShortName,
			LogoSourceUrl: optText(t.LogoURL),
			CountryID:     country,
		})
		if err != nil {
			w.logger.Error("upsert team failed", slog.String("team", t.Name), slog.Any("error", err))
			continue
		}
		ids[t.FlashscoreID] = id
	}
	return ids
}

func (w *Worker) upsertMatch(ctx context.Context, m domain.Match, competition pgtype.UUID, teamIDs map[string]pgtype.UUID) {
	homeID, okHome := teamIDs[m.HomeTeamFlashscoreID]
	awayID, okAway := teamIDs[m.AwayTeamFlashscoreID]
	if !okHome || !okAway {
		w.logger.Warn("skip match: team not stored", slog.String("match", m.FlashscoreID))
		return
	}

	_, err := w.db.UpsertMatch(ctx, db.UpsertMatchParams{
		FlashscoreID:   m.FlashscoreID,
		CompetitionID:  competition,
		Round:          optText(m.Round),
		Phase:          optText(m.Phase),
		Season:         int16(m.Season),
		HomeTeamID:     homeID,
		AwayTeamID:     awayID,
		Status:         m.Status,
		MatchTime:      pgtype.Timestamptz{Time: m.MatchTime, Valid: !m.MatchTime.IsZero()},
		HomeScore:      pgtype.Int2{Int16: int16(m.HomeScore), Valid: m.Status != "scheduled"},
		AwayScore:      pgtype.Int2{Int16: int16(m.AwayScore), Valid: m.Status != "scheduled"},
		Stage:          pgtype.Int2{Int16: int16(m.Stage), Valid: m.Stage != 0},
		StageStartedAt: pgtype.Timestamptz{Time: m.StageStartedAt, Valid: !m.StageStartedAt.IsZero()},
	})
	if err != nil {
		w.logger.Error("upsert match failed", slog.String("match", m.FlashscoreID), slog.Any("error", err))
	}
}

// slugify turns a region name into a slug: "Asia" -> "asia", "North & Central America" -> "north-central-america".
func slugify(s string) string {
	return strings.Join(strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}), "-")
}

type statJSON struct {
	Label string `json:"label"`
	Home  string `json:"home"`
	Away  string `json:"away"`
}

func (w *Worker) syncDetails(ctx context.Context) error {
	if w.detailsPerRun == 0 {
		return nil
	}

	pending, err := w.db.ListMatchesMissingDetails(ctx, int32(w.detailsPerRun))
	if err != nil {
		return fmt.Errorf("list matches missing details: %w", err)
	}

	for _, m := range pending {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.syncMatchDetails(ctx, m); err != nil {
			// ponytail: a match Flashscore never serves details for is retried every run; add an attempts column if that shows up in logs.
			w.logger.Error("sync match details failed", slog.String("match", m.FlashscoreID), slog.Any("error", err))
		}
	}

	return nil
}

func (w *Worker) syncMatchDetails(ctx context.Context, m db.ListMatchesMissingDetailsRow) error {
	events, stats, players, err := flashscore.ScrapeMatchDetails(m.FlashscoreID)
	if err != nil {
		return err
	}

	if err := w.saveEvents(ctx, m.ID, m.HomeTeamID, m.AwayTeamID, events); err != nil {
		return err
	}

	for _, p := range players {
		number := pgtype.Int2{Int16: int16(p.ShirtNumber), Valid: p.ShirtNumber > 0}
		position := pgtype.Text{String: "GK", Valid: p.Goalkeeper}
		team := sideTeam(p.Team, m.HomeTeamID, m.AwayTeamID)

		playerID, err := w.db.UpsertPlayer(ctx, db.UpsertPlayerParams{
			FlashscoreID:   p.FlashscoreID,
			TeamID:         team,
			Name:           p.Name,
			Nationality:    optText(p.Nationality),
			ShirtNumber:    number,
			Position:       position,
			PhotoSourceUrl: optText(p.PhotoURL),
		})
		if err != nil {
			return fmt.Errorf("upsert player %s: %w", p.FlashscoreID, err)
		}
		if err := w.db.UpsertLineup(ctx, db.UpsertLineupParams{
			MatchID: m.ID, PlayerID: playerID, TeamID: team, ShirtNumber: number, Starter: p.Starter,
		}); err != nil {
			return fmt.Errorf("upsert lineup %s: %w", p.FlashscoreID, err)
		}
	}

	out := make([]statJSON, 0, len(stats))
	for _, s := range stats {
		out = append(out, statJSON{Label: s.Name, Home: s.Home, Away: s.Away})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}

	// Stats go last: their row marks the match as done, so a failure above retries it.
	return w.db.UpsertMatchStats(ctx, db.UpsertMatchStatsParams{MatchID: m.ID, Stats: raw})
}

// SyncLive refreshes score, stage, and events of matches in play. It is cheap when
// nothing is live: one indexed query and no requests to Flashscore.
func (w *Worker) SyncLive(ctx context.Context) error {
	candidates, err := w.db.ListLiveCandidates(ctx)
	if err != nil {
		return fmt.Errorf("list live candidates: %w", err)
	}

	for _, m := range candidates {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.syncLiveMatch(ctx, m); err != nil {
			w.logger.Error("sync live match failed", slog.String("match", m.FlashscoreID), slog.Any("error", err))
		}
	}

	return nil
}

func (w *Worker) syncLiveMatch(ctx context.Context, m db.ListLiveCandidatesRow) error {
	state, err := flashscore.ScrapeLive(m.FlashscoreID)
	if err != nil {
		return err
	}

	if state.Status != "scheduled" {
		events, err := flashscore.ScrapeEvents(m.FlashscoreID)
		if err != nil {
			return err
		}
		if err := w.saveEvents(ctx, m.ID, m.HomeTeamID, m.AwayTeamID, events); err != nil {
			return err
		}
	}

	return w.db.UpdateMatchLive(ctx, db.UpdateMatchLiveParams{
		ID:             m.ID,
		Status:         state.Status,
		Stage:          pgtype.Int2{Int16: int16(state.Stage), Valid: state.Stage != 0},
		StageStartedAt: pgtype.Timestamptz{Time: state.StageStartedAt, Valid: !state.StageStartedAt.IsZero()},
		HomeScore:      pgtype.Int2{Int16: int16(state.HomeScore), Valid: state.Status != "scheduled"},
		AwayScore:      pgtype.Int2{Int16: int16(state.AwayScore), Valid: state.Status != "scheduled"},
	})
}

func (w *Worker) saveEvents(ctx context.Context, matchID, homeID, awayID pgtype.UUID, events []flashscore.MatchEvent) error {
	for _, e := range events {
		team := sideTeam(e.Team, homeID, awayID)
		if err := w.db.UpsertMatchEvent(ctx, db.UpsertMatchEventParams{
			MatchID:             matchID,
			FlashscoreID:        e.ID,
			Type:                e.Type,
			Minute:              e.Minute,
			PlayerName:          e.PlayerName,
			TeamID:              team,
			PlayerFlashscoreID:  optText(e.PlayerID),
			RelatedPlayerName:   optText(e.RelatedName),
			RelatedFlashscoreID: optText(e.RelatedID),
		}); err != nil {
			return fmt.Errorf("upsert event %s: %w", e.ID, err)
		}
	}
	return nil
}

// sideTeam maps Flashscore's side (1 home, 2 away) to the team id.
func sideTeam(side int, homeID, awayID pgtype.UUID) pgtype.UUID {
	if side == 2 {
		return awayID
	}
	return homeID
}

func optText(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}
