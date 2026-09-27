package standing

import (
	"context"
	"os"
	"slices"
	"testing"

	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"

	"github.com/jackc/pgx/v5"
)

// Needs a migrated database at TEST_DATABASE_URL; everything runs in a rolled-back transaction.
func TestIntegrationListStandings(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	_, err = tx.Exec(ctx, `
		INSERT INTO teams (id, flashscore_id, name, short_name) VALUES
		  ('00000000-0000-0000-0000-00000000000a', 't-a', 'Alpha', 'ALP'),
		  ('00000000-0000-0000-0000-00000000000b', 't-b', 'Bravo', 'BRA'),
		  ('00000000-0000-0000-0000-00000000000c', 't-c', 'Charlie', 'CHA');
		INSERT INTO matches (flashscore_id, season, home_team_id, away_team_id, status, match_time, home_score, away_score) VALUES
		  ('m1', 1999, '00000000-0000-0000-0000-00000000000a', '00000000-0000-0000-0000-00000000000b', 'finished', '1999-08-01', 2, 0),
		  ('m2', 1999, '00000000-0000-0000-0000-00000000000b', '00000000-0000-0000-0000-00000000000a', 'finished', '1999-08-08', 1, 1),
		  ('m3', 1999, '00000000-0000-0000-0000-00000000000b', '00000000-0000-0000-0000-00000000000c', 'finished', '1999-08-15', 3, 1),
		  ('m4', 1999, '00000000-0000-0000-0000-00000000000c', '00000000-0000-0000-0000-00000000000a', 'scheduled', '1999-08-22', NULL, NULL),
		  ('m5', 1998, '00000000-0000-0000-0000-00000000000c', '00000000-0000-0000-0000-00000000000a', 'finished', '1998-08-22', 9, 0)`)
	if err != nil {
		t.Fatal(err)
	}

	rows, err := db.New(tx).ListStandings(ctx, 1999)
	if err != nil {
		t.Fatal(err)
	}

	type got struct {
		name                        string
		p, w, d, l, gf, ga, points int32
		form                        []string
	}
	want := []got{
		{"Alpha", 2, 1, 1, 0, 3, 1, 4, []string{"D", "W"}},
		{"Bravo", 3, 1, 1, 1, 4, 4, 4, []string{"W", "D", "L"}},
		{"Charlie", 1, 0, 0, 1, 1, 3, 0, []string{"L"}},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %d, want %d", len(rows), len(want))
	}
	for i, r := range rows {
		g := got{r.Name, r.Played, r.Won, r.Drawn, r.Lost, r.GoalsFor, r.GoalsAgainst, r.Points, r.Form}
		w := want[i]
		if g.name != w.name || g.p != w.p || g.w != w.w || g.d != w.d || g.l != w.l ||
			g.gf != w.gf || g.ga != w.ga || g.points != w.points || !slices.Equal(g.form, w.form) {
			t.Errorf("row %d = %+v, want %+v", i+1, g, w)
		}
	}
}
