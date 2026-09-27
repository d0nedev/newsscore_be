package flashscore

import (
	"testing"
	"time"
)

// Trimmed from real feeds captured on 2026-09-27.
const (
	liveMatchRecord = "AA÷noVlM9kd¬AD÷1790485200¬AB÷2¬AC÷13¬AO÷1790489011¬WM÷DER¬PX÷nqL74t6i¬AE÷Deren¬AG÷3¬WN÷AMT¬PY÷pSz559iK¬AF÷Amtat¬AH÷1¬~"
	fixtureRecord   = "AA÷Sl4FtkjU¬AD÷1791534600¬AB÷1¬AC÷1¬WM÷PER¬PX÷MDFOmQ7H¬AE÷Persik Kediri¬OA÷pbvGiliT-fudV7NWp.png¬WN÷MAD¬PY÷6i1jCnP9¬AF÷Madura United¬~"
	liveCoreFeed    = "DA÷2¬DZ÷2¬DB÷13¬DD÷1790489011¬DC÷1790485200¬DE÷3¬DF÷1¬DG÷3¬DH÷1¬~"
)

func TestParseMatch(t *testing.T) {
	records := parseRecords(liveMatchRecord + fixtureRecord)
	if len(records) != 2 {
		t.Fatalf("records = %d, want 2", len(records))
	}

	live, home, away, ok := parseMatch(records[0])
	if !ok || live.Status != "live" || live.Stage != 13 || live.HomeScore != 3 || live.AwayScore != 1 {
		t.Errorf("live match = %+v", live)
	}
	if !live.StageStartedAt.Equal(time.Unix(1790489011, 0)) {
		t.Errorf("stage started = %v", live.StageStartedAt)
	}
	if home.FlashscoreID != "nqL74t6i" || away.Name != "Amtat" {
		t.Errorf("teams = %+v %+v", home, away)
	}

	fixture, _, _, _ := parseMatch(records[1])
	if fixture.Status != "scheduled" || !fixture.StageStartedAt.IsZero() {
		t.Errorf("fixture = %+v", fixture)
	}
}

func TestParseLive(t *testing.T) {
	got := parseLive(parseRecords(liveCoreFeed)[0])
	want := LiveState{Status: "live", Stage: 13, StageStartedAt: time.Unix(1790489011, 0), HomeScore: 3, AwayScore: 1}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}
