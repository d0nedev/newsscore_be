package league

import (
	"testing"

	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestSplitGroups(t *testing.T) {
	id := func(b byte) pgtype.UUID { return pgtype.UUID{Bytes: [16]byte{15: b}, Valid: true} }
	row := func(b byte, name string) db.ListStandingsRow { return db.ListStandingsRow{ID: id(b), Name: name} }

	// Ranked overall: 3, 1, 4, 2. Group {1,2} and group {3,4}.
	rows := []db.ListStandingsRow{row(3, "C"), row(1, "A"), row(4, "D"), row(2, "B")}
	pairings := []db.ListGroupPairingsRow{
		{HomeTeamID: id(1), AwayTeamID: id(2)},
		{HomeTeamID: id(4), AwayTeamID: id(3)},
	}

	groups := splitGroups(rows, pairings)
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	want := [][]string{{"C", "D"}, {"A", "B"}}
	for i, g := range groups {
		if g.Name != "Grup "+string(rune('A'+i)) {
			t.Errorf("group %d name = %q", i, g.Name)
		}
		for j, r := range g.Standings {
			if r.Team != want[i][j] || r.Position != j+1 {
				t.Errorf("group %d row %d = %s #%d, want %s #%d", i, j, r.Team, r.Position, want[i][j], j+1)
			}
		}
	}
}
