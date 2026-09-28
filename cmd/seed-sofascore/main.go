// Command seed-sofascore loads Indonesian competitions that Flashscore lacks
// (Liga Nusantara, EPA, Liga Topskor) from SofaScore pages saved by
// scrap-claude/sofascore_spike.py --ssr: competition, table, teams, and squads.
// It reads files only; fetching them is left to that script.
//
//	python3 scrap-claude/sofascore_spike.py --ssr /football/tournament/indonesia/indonesia-liga-3/24432
//	go run ./cmd/seed-sofascore -dir scrap-claude/sofascore_spike
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/d0nedev/newsscore/internal/platform/config"
	"github.com/d0nedev/newsscore/internal/platform/database"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// onFlashscore are SofaScore ids of competitions the ingestor already scrapes
// from Flashscore: Super League, Championship, Piala Presiden.
var onFlashscore = map[int]bool{1015: true, 23229: true, 23013: true}

func main() {
	dir := flag.String("dir", "scrap-claude/sofascore_spike", "folder with ssr_tournament_*.json and ssr_team_*.json")
	flag.Parse()
	if err := run(*dir); err != nil {
		slog.Error("seed failed", slog.Any("error", err))
		os.Exit(1)
	}
}

func run(dir string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := database.NewPostgresPool(ctx, cfg.DB)
	if err != nil {
		return err
	}
	defer pool.Close()

	files, err := filepath.Glob(filepath.Join(dir, "ssr_tournament_*.json"))
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no ssr_tournament_*.json in %s", dir)
	}
	for _, f := range files {
		var page tournamentPage
		if err := readJSON(f, &page); err != nil {
			return err
		}
		ut := page.UniqueTournament
		switch {
		case ut.Category.Country.Alpha2 != "ID":
			slog.Info("skip: not Indonesian", slog.String("competition", ut.Name))
			continue
		case onFlashscore[ut.ID]:
			slog.Info("skip: scraped from Flashscore", slog.String("competition", ut.Name))
			continue
		}

		// One transaction per competition: a bad file leaves nothing half-seeded.
		tx, err := pool.Begin(ctx)
		if err != nil {
			return err
		}
		teams, players, err := seed(ctx, db.New(tx), dir, page)
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("%s: %w", ut.Name, err)
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
		slog.Info("seeded", slog.String("competition", ut.Name), slog.Int("season", int(page.season())),
			slog.Int("teams", teams), slog.Int("players", players))
	}
	return nil
}

func seed(ctx context.Context, q *db.Queries, dir string, page tournamentPage) (teams, players int, err error) {
	ut := page.UniqueTournament
	country, err := q.UpsertCountry(ctx, db.UpsertCountryParams{Slug: "indonesia", Name: "Indonesia"})
	if err != nil {
		return 0, 0, err
	}
	kind := "league"
	if len(page.Standings) > 1 {
		kind = "cup" // one table per group
	}
	competition, err := q.UpsertSofascoreCompetition(ctx, db.UpsertSofascoreCompetitionParams{
		Slug: ut.Slug, Name: ut.Name, CountryID: country, Type: kind,
		SofascoreID: pgtype.Int4{Int32: int32(ut.ID), Valid: true},
	})
	if err != nil {
		return 0, 0, fmt.Errorf("competition: %w", err)
	}

	season := page.season()
	if err := q.DeleteStandingSnapshot(ctx, db.DeleteStandingSnapshotParams{CompetitionID: competition, Season: season}); err != nil {
		return 0, 0, err
	}
	seen := map[int]bool{} // a team in several stages is stored and counted once
	for order, g := range page.Standings {
		group := groupName(g.Tournament.Name, ut.Name)
		for _, r := range g.Rows {
			short := firstNonEmpty(r.Team.ShortName, r.Team.NameCode, r.Team.Name)
			team, err := q.UpsertSofascoreTeam(ctx, db.UpsertSofascoreTeamParams{
				SofascoreID: pgtype.Int4{Int32: int32(r.Team.ID), Valid: true}, Name: r.Team.Name, ShortName: short,
			})
			if err != nil {
				return 0, 0, fmt.Errorf("team %s: %w", r.Team.Name, err)
			}
			if err := q.InsertStandingSnapshot(ctx, db.InsertStandingSnapshotParams{
				CompetitionID: competition, Season: season, GroupName: group, GroupOrder: int16(order), Position: int16(r.Position), TeamID: team,
				Played: int16(r.Matches), Won: int16(r.Wins), Drawn: int16(r.Draws), Lost: int16(r.Losses),
				GoalsFor: int16(r.ScoresFor), GoalsAgainst: int16(r.ScoresAgainst), Points: int16(r.Points),
			}); err != nil {
				return 0, 0, fmt.Errorf("standing %s: %w", r.Team.Name, err)
			}
			if seen[r.Team.ID] {
				continue
			}
			seen[r.Team.ID] = true
			teams++
			n, err := seedSquad(ctx, q, dir, r.Team.ID, team)
			if err != nil {
				return 0, 0, fmt.Errorf("squad %s: %w", r.Team.Name, err)
			}
			players += n
		}
	}
	return teams, players, nil
}

// seedSquad stores a team's squad from ssr_team_<id>.json; a missing file is logged, not fatal.
func seedSquad(ctx context.Context, q *db.Queries, dir string, sofascoreTeam int, team pgtype.UUID) (int, error) {
	var page teamPage
	f := filepath.Join(dir, "ssr_team_"+strconv.Itoa(sofascoreTeam)+".json")
	if err := readJSON(f, &page); err != nil {
		if os.IsNotExist(err) {
			slog.Warn("no squad file", slog.String("file", f))
			return 0, nil
		}
		return 0, err
	}
	for _, s := range page.Players.Players {
		p := s.Player
		var born pgtype.Date
		if p.DateOfBirthTimestamp != 0 {
			born = pgtype.Date{Time: time.Unix(p.DateOfBirthTimestamp, 0).UTC(), Valid: true}
		}
		if err := q.UpsertSofascorePlayer(ctx, db.UpsertSofascorePlayerParams{
			SofascoreID: pgtype.Int4{Int32: int32(p.ID), Valid: true},
			TeamID:      team,
			Name:        p.Name,
			Nationality: pgtype.Text{String: p.Country.Name, Valid: p.Country.Name != ""},
			Position:    pgtype.Text{String: positions[p.Position], Valid: positions[p.Position] != ""},
			BirthDate:   born,
			HeightCm:    pgtype.Int2{Int16: int16(p.Height), Valid: p.Height > 0},
		}); err != nil {
			return 0, fmt.Errorf("player %s: %w", p.Name, err)
		}
	}
	return len(page.Players.Players), nil
}

var positions = map[string]string{"G": "GK", "D": "DF", "M": "MF", "F": "FW"}

// groupName is a table's name without the competition's: "Liga Topskor U17,
// Second Round, Group A" -> "Second Round, Group A". A single table is named
// after the competition and has no group; "3rd Place" has no prefix at all.
func groupName(table, competition string) string {
	table = strings.TrimSpace(table)
	if table == competition {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(table, competition+", "))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func readJSON(file string, v any) error {
	b, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("%s: %w", file, err)
	}
	return nil
}

// tournamentPage is the pageProps the spike saves for a competition page.
type tournamentPage struct {
	UniqueTournament struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Slug     string `json:"slug"`
		Category struct {
			Country struct {
				Alpha2 string `json:"alpha2"`
			} `json:"country"`
		} `json:"category"`
	} `json:"uniqueTournament"`
	Seasons []struct {
		Year string `json:"year"` // "25/26" or "2026"
	} `json:"seasons"`
	Standings []struct {
		Tournament struct {
			Name string `json:"name"`
		} `json:"tournament"`
		Rows []struct {
			Position      int `json:"position"`
			Matches       int `json:"matches"`
			Wins          int `json:"wins"`
			Draws         int `json:"draws"`
			Losses        int `json:"losses"`
			ScoresFor     int `json:"scoresFor"`
			ScoresAgainst int `json:"scoresAgainst"`
			Points        int `json:"points"`
			Team          struct {
				ID        int    `json:"id"`
				Name      string `json:"name"`
				ShortName string `json:"shortName"`
				NameCode  string `json:"nameCode"`
			} `json:"team"`
		} `json:"rows"`
	} `json:"standings"`
}

// season is the latest season's first year, like Flashscore's: "25/26" -> 2025.
func (p tournamentPage) season() int16 {
	if len(p.Seasons) == 0 {
		return 0
	}
	first, _, _ := strings.Cut(p.Seasons[0].Year, "/")
	n, _ := strconv.Atoi(first)
	if n < 100 {
		n += 2000
	}
	return int16(n)
}

type teamPage struct {
	Players struct {
		Players []struct {
			Player struct {
				ID                   int    `json:"id"`
				Name                 string `json:"name"`
				Position             string `json:"position"` // G, D, M, F
				Height               int    `json:"height"`
				DateOfBirthTimestamp int64  `json:"dateOfBirthTimestamp"`
				Country              struct {
					Name string `json:"name"`
				} `json:"country"`
			} `json:"player"`
		} `json:"players"`
	} `json:"players"`
}
