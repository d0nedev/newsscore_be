package match

import (
	"net/url"
	"testing"
	"time"

	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestParseListFilter(t *testing.T) {
	f, err := parseListFilter(url.Values{"date": {"14.09.2026"}, "status": {"live"}})
	if err != nil {
		t.Fatal(err)
	}
	// 14.09.2026 00:00 WIB is 13.09.2026 17:00 UTC.
	if want := time.Date(2026, 9, 13, 17, 0, 0, 0, time.UTC); !f.From.Equal(want) {
		t.Errorf("From = %v, want %v", f.From.UTC(), want)
	}

	for _, bad := range []url.Values{
		{"date": {"2026-09-14"}},
		{"status": {"done"}},
		{"teamId": {"x"}},
	} {
		if _, err := parseListFilter(bad); err == nil {
			t.Errorf("%v: want error", bad)
		}
	}
}

func TestToMatchResponse(t *testing.T) {
	kickoff := time.Date(2026, 9, 14, 12, 30, 0, 0, time.UTC)
	row := db.ListMatchesRow{
		Status:    "scheduled",
		MatchTime: pgtype.Timestamptz{Time: kickoff, Valid: true},
		HomeScore: pgtype.Int2{Valid: true},
		AwayScore: pgtype.Int2{Valid: true},
	}

	got := toMatchResponse(row)
	if got.Time != "19:30" || got.Date != "14.09.2026" || got.Score != nil {
		t.Errorf("scheduled: got time=%q date=%q score=%v", got.Time, got.Date, got.Score)
	}

	row.Status, row.HomeScore.Int16, row.AwayScore.Int16 = "finished", 2, 1
	if got := toMatchResponse(row); got.Score == nil || *got.Score != [2]int{2, 1} {
		t.Errorf("finished score = %v", got.Score)
	}
}

func TestEventType(t *testing.T) {
	for raw, want := range map[string]string{
		"Goal": "goal", "Yellow Card": "yellow", "Red Card": "red",
		"Substitution - In": "sub", "Penalty Awarded": "penalty awarded",
	} {
		if got := eventType(raw); got != want {
			t.Errorf("eventType(%q) = %q, want %q", raw, got, want)
		}
	}
}
