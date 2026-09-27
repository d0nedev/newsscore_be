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
	row := db.MatchRow{
		Status:    "scheduled",
		MatchTime: pgtype.Timestamptz{Time: kickoff, Valid: true},
		HomeScore: pgtype.Int2{Valid: true},
		AwayScore: pgtype.Int2{Valid: true},
	}

	got := ToMatchResponse(row)
	if got.Time != "19:30" || got.Date != "14.09.2026" || got.Score != nil {
		t.Errorf("scheduled: got time=%q date=%q score=%v", got.Time, got.Date, got.Score)
	}

	row.Status, row.HomeScore.Int16, row.AwayScore.Int16 = "finished", 2, 1
	if got := ToMatchResponse(row); got.Score == nil || *got.Score != [2]int{2, 1} {
		t.Errorf("finished score = %v", got.Score)
	}
}

func TestEventType(t *testing.T) {
	for raw, want := range map[string]string{
		"Goal": "goal", "Yellow Card": "yellow", "Red Card": "red",
		"Substitution - In": "sub", "Penalty": "goal", "Assistance": "",
		"Not on pitch": "not on pitch",
	} {
		if got := eventType(raw); got != want {
			t.Errorf("eventType(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestLiveMinute(t *testing.T) {
	start := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	at := func(min int) time.Time { return start.Add(time.Duration(min)*time.Minute + 30*time.Second) }
	started := pgtype.Timestamptz{Time: start, Valid: true}
	stage := func(v int16) pgtype.Int2 { return pgtype.Int2{Int16: v, Valid: true} }

	tests := []struct {
		stage pgtype.Int2
		now   time.Time
		want  string
	}{
		{stage(stageFirstHalf), at(0), "1'"},
		{stage(stageFirstHalf), at(44), "45'"},
		{stage(stageFirstHalf), at(47), "45+'"},
		{stage(stageHalfTime), at(50), "HT"},
		{stage(stageSecondHalf), at(21), "67'"},
		{stage(stageSecondHalf), at(48), "90+'"},
		{stage(6), at(5), "LIVE"},
		{pgtype.Int2{}, at(5), "LIVE"},
	}
	for _, tt := range tests {
		if got := liveMinute(tt.stage, started, tt.now); got != tt.want {
			t.Errorf("stage %d at %v: got %q, want %q", tt.stage.Int16, tt.now.Sub(start), got, tt.want)
		}
	}
}

func TestRenderNotification(t *testing.T) {
	started := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	stage, home, away := int16(stageSecondHalf), int16(2), int16(1)

	event, data := render(notification{
		Kind: "score", ID: "m1", Status: "live",
		Stage: &stage, StageStartedAt: &started, HomeScore: &home, AwayScore: &away,
	}, started.Add(21*time.Minute+30*time.Second))
	u, _ := data.(ScoreUpdate)
	if event != "score" || u.Time != "67'" || u.Score == nil || *u.Score != [2]int{2, 1} {
		t.Errorf("score = %s %+v", event, u)
	}

	if event, _ := render(notification{Kind: "match_event", Type: "Assistance"}, started); event != "" {
		t.Errorf("assistance should be dropped, got %q", event)
	}
}
