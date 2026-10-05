package ingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/provider/sofascore"

	"github.com/jackc/pgx/v5/pgtype"
)

// sofascoreEvery spaces SofaScore league syncs: one costs ~10 requests per
// competition, and these youth leagues change at most daily.
// ponytail: in-memory, so a restart syncs at once; a column if restarts get frequent.
const sofascoreEvery = time.Hour

// squadsPerRunSS caps SofaScore squad reads per sync; each team is read weekly.
const squadsPerRunSS = 20

// syncSofascore refreshes every SofaScore competition (matches, table, top
// players) at most hourly, and a few squads. Match details go through jobs.
// A 403 stops the run until next hour: SofaScore asks for a captcha once it
// sees too many requests, and more of them only keep it asking.
func (w *Worker) syncSofascore(ctx context.Context) error {
	if time.Since(w.sofascoreSynced) < sofascoreEvery {
		return nil
	}
	w.sofascoreSynced = time.Now()
	competitions, err := w.db.ListSofascoreCompetitions(ctx)
	if err != nil {
		return fmt.Errorf("list sofascore competitions: %w", err)
	}
	for _, c := range competitions {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := w.syncSofascoreCompetition(ctx, c)
		if errors.Is(err, sofascore.ErrBlocked) {
			return err
		}
		if err != nil {
			w.logger.Error("sync sofascore competition failed", slog.String("competition", c.Slug), slog.Any("error", err))
		}
	}
	return w.syncSofascoreSquads(ctx, squadsPerRunSS)
}

func (w *Worker) syncSofascoreCompetition(ctx context.Context, c db.ListSofascoreCompetitionsRow) error {
	season, err := sofascore.LatestSeason(int(c.SofascoreID))
	if err != nil {
		return err
	}
	teams := map[int]pgtype.UUID{}
	team := func(t sofascore.Team) (pgtype.UUID, error) {
		if id, ok := teams[t.ID]; ok {
			return id, nil
		}
		id, err := w.db.UpsertSofascoreTeam(ctx, db.UpsertSofascoreTeamParams{
			SofascoreID: pgtype.Int4{Int32: int32(t.ID), Valid: true}, Name: t.Name, ShortName: t.Short(),
		})
		if err != nil {
			return id, fmt.Errorf("team %s: %w", t.Name, err)
		}
		teams[t.ID] = id
		return id, nil
	}

	events, err := sofascore.Events(int(c.SofascoreID), season.ID)
	if err != nil {
		return fmt.Errorf("events: %w", err)
	}
	for _, e := range events {
		home, err := team(e.HomeTeam)
		if err != nil {
			return err
		}
		away, err := team(e.AwayTeam)
		if err != nil {
			return err
		}
		if err := w.db.UpsertSofascoreMatch(ctx, db.UpsertSofascoreMatchParams{
			SofascoreID:   pgtype.Int4{Int32: int32(e.ID), Valid: true},
			CompetitionID: c.ID,
			Season:        season.Year,
			HomeTeamID:    home,
			AwayTeamID:    away,
			Status:        e.MatchStatus(),
			MatchTime:     pgtype.Timestamptz{Time: time.Unix(e.StartTimestamp, 0), Valid: e.StartTimestamp != 0},
			HomeScore:     score(e.HomeScore),
			AwayScore:     score(e.AwayScore),
			Round:         optText(e.Round()),
		}); err != nil {
			return fmt.Errorf("match %d: %w", e.ID, err)
		}
	}

	tables, err := sofascore.Standings(int(c.SofascoreID), season.ID)
	if err != nil {
		return fmt.Errorf("standings: %w", err)
	}
	if len(tables) > 0 {
		if err := w.db.DeleteStandingSnapshot(ctx, db.DeleteStandingSnapshotParams{CompetitionID: c.ID, Season: season.Year}); err != nil {
			return err
		}
		for order, t := range tables {
			for _, r := range t.Rows {
				id, err := team(r.Team)
				if err != nil {
					return err
				}
				if err := w.db.InsertStandingSnapshot(ctx, db.InsertStandingSnapshotParams{
					CompetitionID: c.ID, Season: season.Year, GroupName: groupName(t.Name, c.Name), GroupOrder: int16(order),
					Position: int16(r.Position), TeamID: id,
					Played: int16(r.Matches), Won: int16(r.Wins), Drawn: int16(r.Draws), Lost: int16(r.Losses),
					GoalsFor: int16(r.ScoresFor), GoalsAgainst: int16(r.ScoresAgainst), Points: int16(r.Points),
				}); err != nil {
					return fmt.Errorf("standing %s: %w", r.Team.Name, err)
				}
			}
		}
	}

	top, err := sofascore.TopPlayers(int(c.SofascoreID), season.ID)
	if err != nil {
		return fmt.Errorf("top players: %w", err)
	}
	if err := w.db.DeleteTopPlayers(ctx, db.DeleteTopPlayersParams{CompetitionID: c.ID, Season: season.Year}); err != nil {
		return err
	}
	for category, list := range top {
		for rank, p := range list {
			teamID, err := team(p.Team)
			if err != nil {
				return err
			}
			playerID, err := w.upsertSofascorePlayer(ctx, p.Player, teamID, 0)
			if err != nil {
				return err
			}
			var value pgtype.Numeric
			if err := value.Scan(string(p.Statistics[category])); err != nil {
				return fmt.Errorf("top %s value %q: %w", category, p.Statistics[category], err)
			}
			var apps int16
			_ = json.Unmarshal(p.Statistics["appearances"], &apps)
			if err := w.db.InsertTopPlayer(ctx, db.InsertTopPlayerParams{
				CompetitionID: c.ID, Season: season.Year, Category: category, Rank: int16(rank + 1),
				PlayerID: playerID, TeamID: teamID, Value: value, Appearances: pgtype.Int2{Int16: apps, Valid: apps > 0},
			}); err != nil {
				return fmt.Errorf("top %s %s: %w", category, p.Player.Name, err)
			}
		}
	}

	w.logger.Info("sofascore competition synced", slog.String("competition", c.Slug), slog.Int("season", int(season.Year)),
		slog.Int("matches", len(events)), slog.Int("tables", len(tables)), slog.Int("top_categories", len(top)))
	return nil
}

// groupName is a table's name without the competition's and the year: "Liga
// Topskor U17, Group A 2026" -> "Group A"; "3rd Place  2026" -> "3rd Place"; the
// competition's only table -> "". Same names as cmd/seed-sofascore stored.
func groupName(table, competition string) string {
	table = strings.TrimSpace(table)
	if i := strings.LastIndexByte(table, ' '); i > 0 {
		if year := table[i+1:]; strings.Trim(year, "0123456789/") == "" { // "2026", "25/26"
			table = strings.TrimSpace(table[:i])
		}
	}
	if table == competition {
		return ""
	}
	return strings.TrimPrefix(table, competition+", ")
}

func score(s sofascore.Score) pgtype.Int2 {
	if s.Current == nil {
		return pgtype.Int2{}
	}
	return pgtype.Int2{Int16: int16(*s.Current), Valid: true}
}

// upsertSofascorePlayer stores a player and returns their id; shirt 0 keeps the known number.
func (w *Worker) upsertSofascorePlayer(ctx context.Context, p sofascore.Player, team pgtype.UUID, shirt int) (pgtype.UUID, error) {
	if shirt == 0 {
		shirt, _ = strconv.Atoi(p.JerseyNumber)
	}
	born := p.Born()
	id, err := w.db.UpsertSofascorePlayer(ctx, db.UpsertSofascorePlayerParams{
		SofascoreID: pgtype.Int4{Int32: int32(p.ID), Valid: true},
		TeamID:      team,
		Name:        p.Name,
		Nationality: optText(p.Country.Name),
		Position:    optText(p.Pos()),
		BirthDate:   pgtype.Date{Time: born, Valid: !born.IsZero()},
		HeightCm:    pgtype.Int2{Int16: int16(p.Height), Valid: p.Height > 0},
		ShirtNumber: pgtype.Int2{Int16: int16(shirt), Valid: shirt > 0},
	})
	if err != nil {
		return id, fmt.Errorf("player %s: %w", p.Name, err)
	}
	return id, nil
}

func (w *Worker) syncSofascoreSquads(ctx context.Context, limit int32) error {
	teams, err := w.db.ListSofascoreSquadsToSync(ctx, limit)
	if err != nil {
		return err
	}
	for _, t := range teams {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		players, err := sofascore.Squad(int(t.SofascoreID))
		// Not marked synced: retried next run. A 404 (no squad listed) is marked, so tried again next week.
		if err != nil && !errors.Is(err, sofascore.ErrNotFound) {
			w.logger.Error("sofascore squad failed", slog.Int("team", int(t.SofascoreID)), slog.Any("error", err))
			continue
		}
		for _, p := range players {
			if _, err := w.upsertSofascorePlayer(ctx, p, t.ID, 0); err != nil {
				return err
			}
		}
		if err := w.db.MarkSquadSynced(ctx, t.ID); err != nil {
			return err
		}
	}
	return nil
}

// BackfillSofascore is a first fill: every SofaScore competition, every squad,
// and every finished match's details now, without the ingestor's per-run caps.
func (w *Worker) BackfillSofascore(ctx context.Context) error {
	w.sofascoreSynced = time.Time{}
	if err := w.syncSofascore(ctx); err != nil {
		return err
	}
	if err := w.syncSofascoreSquads(ctx, 10000); err != nil {
		return err
	}
	matches, err := w.db.ListSofascoreMatchesWithoutDetails(ctx)
	if err != nil {
		return err
	}
	for i, id := range matches {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := w.syncSofascoreMatch(ctx, strconv.Itoa(int(id)))
		if errors.Is(err, sofascore.ErrBlocked) {
			return err // the ingestor's jobs pick up the rest later
		}
		if err != nil {
			w.logger.Warn("sofascore match failed", slog.Int("event", int(id)), slog.Any("error", err))
		}
		if (i+1)%50 == 0 {
			w.logger.Info("sofascore backfill", slog.Int("done", i+1), slog.Int("total", len(matches)))
		}
	}
	return nil
}

// syncSofascoreMatch stores a finished match's incidents, lineups with each
// player's numbers, and team statistics.
func (w *Worker) syncSofascoreMatch(ctx context.Context, target string) error {
	eventID, err := strconv.Atoi(target)
	if err != nil {
		return err
	}
	m, err := w.db.GetMatchBySofascoreID(ctx, pgtype.Int4{Int32: int32(eventID), Valid: true})
	if err != nil {
		return fmt.Errorf("get match: %w", err)
	}
	d, err := sofascore.MatchDetails(eventID)
	if err != nil {
		return err
	}

	side := func(home bool) pgtype.UUID {
		if home {
			return m.HomeTeamID
		}
		return m.AwayTeamID
	}
	for _, inc := range d.Incidents {
		e, ok := inc.Event()
		if !ok {
			continue
		}
		if err := w.db.UpsertSofascoreMatchEvent(ctx, db.UpsertSofascoreMatchEventParams{
			MatchID:            m.ID,
			FlashscoreID:       e.ID,
			Type:               e.Type,
			Minute:             e.Minute,
			PlayerName:         e.Player.Name,
			TeamID:             side(e.Home),
			PlayerSofascoreID:  optInt(e.Player.ID),
			RelatedPlayerName:  optText(e.Related.Name),
			RelatedSofascoreID: optInt(e.Related.ID),
		}); err != nil {
			return fmt.Errorf("upsert event %s: %w", e.ID, err)
		}
	}

	for home, players := range map[bool][]sofascore.LineupPlayer{true: d.Home, false: d.Away} {
		team := side(home)
		for _, p := range players {
			playerID, err := w.upsertSofascorePlayer(ctx, p.Player, team, p.ShirtNumber)
			if err != nil {
				return err
			}
			if err := w.db.UpsertSofascoreLineup(ctx, db.UpsertSofascoreLineupParams{
				MatchID: m.ID, PlayerID: playerID, TeamID: team,
				ShirtNumber: pgtype.Int2{Int16: int16(p.ShirtNumber), Valid: p.ShirtNumber > 0},
				Starter:     !p.Substitute,
				Stats:       playerStats(p.Statistics),
			}); err != nil {
				return fmt.Errorf("upsert lineup %s: %w", p.Player.Name, err)
			}
		}
	}

	out := make([]statJSON, 0, len(d.Stats))
	for _, s := range d.Stats {
		out = append(out, statJSON{Label: s.Name, Home: s.Home, Away: s.Away})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return err
	}
	// Stats go last: their row marks the match as done, so a failure above retries it.
	return w.db.UpsertMatchStats(ctx, db.UpsertMatchStatsParams{MatchID: m.ID, Stats: raw})
}

// playerStats drops SofaScore's type tag; nil when nothing else is left.
func playerStats(raw json.RawMessage) []byte {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return nil
	}
	delete(m, "statisticsType")
	if len(m) == 0 {
		return nil
	}
	b, _ := json.Marshal(m)
	return b
}

func optInt(n int) pgtype.Int4 {
	return pgtype.Int4{Int32: int32(n), Valid: n != 0}
}
