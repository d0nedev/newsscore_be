package ingest

import (
	"context"
	"log"
	"sort"

	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"
	"github.com/d0nedev/newsscore/internal/provider/flashscore"
	"github.com/jackc/pgx/v5/pgtype"
)

type Worker struct {
	db *db.Queries
}

func NewWorker(queries *db.Queries) *Worker {
	return &Worker{db: queries}
}

func (w *Worker) StartDailySync(ctx context.Context) {
	log.Println("Memulai Ingestor Harian (Daily Sync)...")

	matches, teams, err := flashscore.ScrapeResults()
	if err != nil {
		log.Printf("Gagal menarik data dari Flashscore: %v\n", err)
		return
	}

	log.Printf("Berhasil mengambil %d tim dan %d pertandingan dari Provider.\n", len(teams), len(matches))

	teamIDs := make(map[string]pgtype.UUID, len(teams))
	for _, t := range teams {
		id, err := w.db.UpsertTeam(ctx, db.UpsertTeamParams{
			FlashscoreID: t.FlashscoreID,
			Name:         t.Name,
			ShortName:    t.ShortName,
			LogoUrl:      pgtype.Text{String: t.LogoURL, Valid: t.LogoURL != ""},
		})
		if err != nil {
			log.Printf("Gagal Upsert Tim %s: %v\n", t.Name, err)
			continue
		}
		teamIDs[t.FlashscoreID] = id
	}

	stats := make(map[pgtype.UUID]*db.UpsertStandingParams)

	for _, m := range matches {
		homeID, okHome := teamIDs[m.HomeTeamFlashscoreID]
		awayID, okAway := teamIDs[m.AwayTeamFlashscoreID]
		if !okHome || !okAway {
			log.Printf("Lewati Match %s: tim belum tersimpan\n", m.FlashscoreID)
			continue
		}
		season := int16(m.Season)

		_, err := w.db.UpsertMatch(ctx, db.UpsertMatchParams{
			FlashscoreID: m.FlashscoreID,
			Season:       season,
			HomeTeamID:   homeID,
			AwayTeamID:   awayID,
			Status:       m.Status,
			MatchTime:    pgtype.Timestamptz{Time: m.MatchTime, Valid: !m.MatchTime.IsZero()},
			HomeScore:    pgtype.Int2{Int16: int16(m.HomeScore), Valid: true},
			AwayScore:    pgtype.Int2{Int16: int16(m.AwayScore), Valid: true},
		})
		if err != nil {
			log.Printf("Gagal Upsert Match %s: %v\n", m.FlashscoreID, err)
		}

		if m.Status == "finished" {
			if stats[homeID] == nil {
				stats[homeID] = &db.UpsertStandingParams{TeamID: homeID, Season: season}
			}
			if stats[awayID] == nil {
				stats[awayID] = &db.UpsertStandingParams{TeamID: awayID, Season: season}
			}

			sh := stats[homeID]
			sa := stats[awayID]

			if m.HomeScore > m.AwayScore {
				sh.Points += 3
			} else if m.HomeScore < m.AwayScore {
				sa.Points += 3
			} else {
				sh.Points += 1
				sa.Points += 1
			}
		}
	}

	var standings []*db.UpsertStandingParams
	for _, v := range stats {
		standings = append(standings, v)
	}

	sort.Slice(standings, func(i, j int) bool {
		return standings[i].Points > standings[j].Points
	})

	for i, st := range standings {
		st.Rank = int16(i + 1)
		err := w.db.UpsertStanding(ctx, *st)
		if err != nil {
			log.Printf("Gagal Upsert Standing %s: %v\n", st.TeamID, err)
		}
	}

	log.Println("Ingestor Harian Selesai! Seluruh data tersimpan ke database.")
}
