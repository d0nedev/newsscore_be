package flashscore

import (
	"testing"
	"time"

	"github.com/d0nedev/newsscore/internal/domain"
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

// Trimmed from df_sui_1_KO07rBLH and df_li_1_KO07rBLH captured on 2026-09-27.
const (
	incidentsFeed = "AC÷1st Half¬IG÷1¬IH÷1¬~" +
		"III÷lCyjzCFd¬IA÷1¬IB÷13'¬IE÷5¬IF÷Pirulo¬IK÷Penalty Awarded¬IM÷MFi0e8JU¬IE÷10¬IF÷Pirulo¬IK÷Penalty¬IM÷MFi0e8JU¬~" +
		"III÷SrGtdup4¬IA÷2¬IB÷21'¬IE÷3¬IF÷Haye T.¬IK÷Goal¬IM÷Y7DW0nHu¬IE÷8¬IF÷Tanamal I.¬IK÷Assistance¬IM÷Sjf4UlCG¬~" +
		"III÷necyDrYH¬IA÷2¬IB÷39'¬IE÷1¬IF÷Haye T.¬IK÷Yellow Card¬IM÷Y7DW0nHu¬~" +
		"III÷OjJnQy65¬IA÷2¬IB÷46'¬IE÷6¬IF÷Putra B.¬IK÷Substitution - Out¬IM÷4bIpHm56¬IE÷7¬IF÷Sekulic B.¬IK÷Substitution - In¬IM÷AB4TE5oN¬~"
	lineupsFeed = "LA÷Formation¬LB÷Starting Lineups¬LGT÷1¬LC÷1¬~" +
		"LD÷1-4-4-2¬LH÷0¬LP÷Y5cFTU5E¬LI÷Chica S.¬LPX÷0MnIUniA-EHOqXBOi.png¬LJ÷44¬LQ÷Spain¬~" +
		"LH÷2¬LP÷OApFFtdg¬LI÷Handika R.¬LR÷(G)¬LS÷Goalkeeper¬LJ÷1¬~" +
		"LC÷2¬~LD÷1-4-2-3-1¬LH÷1¬LP÷Y7DW0nHu¬LI÷Haye T.¬LJ÷33¬~" +
		"LB÷Substitutes¬LGT÷1¬LC÷1¬~LH÷11¬LP÷SMRZSrKm¬LI÷Abizal R.¬LJ÷1¬LQ÷Indonesia¬~" +
		"LB÷Coaches¬LGT÷4¬LC÷1¬~LH÷22¬LP÷xrnNzeyc¬LI÷Lemos M.¬LJ÷¬~"
)

func TestParseIncidents(t *testing.T) {
	got := parseIncidents(incidentsFeed)
	want := []MatchEvent{
		{ID: "lCyjzCFd", Team: 1, Minute: "13'", Type: "Penalty", PlayerName: "Pirulo", PlayerID: "MFi0e8JU"},
		{ID: "SrGtdup4", Team: 2, Minute: "21'", Type: "Goal", PlayerName: "Haye T.", PlayerID: "Y7DW0nHu", RelatedName: "Tanamal I.", RelatedID: "Sjf4UlCG"},
		{ID: "necyDrYH", Team: 2, Minute: "39'", Type: "Yellow Card", PlayerName: "Haye T.", PlayerID: "Y7DW0nHu"},
		{ID: "OjJnQy65", Team: 2, Minute: "46'", Type: "Substitution", PlayerName: "Sekulic B.", PlayerID: "AB4TE5oN", RelatedName: "Putra B.", RelatedID: "4bIpHm56"},
	}
	if len(got) != len(want) {
		t.Fatalf("events = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestParseLineups(t *testing.T) {
	got := parseLineups(lineupsFeed)
	want := []Player{
		{FlashscoreID: "Y5cFTU5E", Name: "Chica S.", Nationality: "Spain", ShirtNumber: 44, Team: 1, Starter: true,
			PhotoURL: "https://static.flashscore.com/res/image/data/0MnIUniA-EHOqXBOi.png"},
		{FlashscoreID: "OApFFtdg", Name: "Handika R.", ShirtNumber: 1, Team: 1, Starter: true, Goalkeeper: true},
		{FlashscoreID: "Y7DW0nHu", Name: "Haye T.", ShirtNumber: 33, Team: 2, Starter: true},
		{FlashscoreID: "SMRZSrKm", Name: "Abizal R.", Nationality: "Indonesia", ShirtNumber: 1, Team: 1},
	}
	if len(got) != len(want) {
		t.Fatalf("players = %d, want %d (coaches must be skipped): %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("player %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func TestSeasonPattern(t *testing.T) {
	for html, want := range map[string]string{
		`<div class="heading__info">2026/2027</div>`: "2026",
		`<div class="heading__info">2026</div>`:      "2026",
	} {
		got := seasonPattern.FindStringSubmatch(html)
		if len(got) < 2 || got[1] != want {
			t.Errorf("%q: got %v, want %s", html, got, want)
		}
	}
}

// A team page quotes feed names with " and lists each match under its competition.
func TestParseFeedsTeamPage(t *testing.T) {
	section := "ZA÷ASIA: ASEAN Championship¬ZK÷ASEAN Championship¬ZL÷/football/asia/asean-championship/¬ZY÷Asia¬~"
	match := "AA÷0hBlRgoL¬AD÷1790341200¬AB÷3¬PX÷88ErHiT9¬AE÷Indonesia¬WU÷indonesia¬AG÷2¬PY÷G0dqs0hU¬AF÷Singapore¬WV÷singapore¬AH÷0¬~"
	html := `cjs.initialFeeds["fixtures"] = { data: ` + "``" + ` };` +
		`cjs.initialFeeds["results"] = { data: ` + "`" + section + match + "`" + ` };`

	matches, teams, err := parseFeeds(html)
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 || len(teams) != 2 {
		t.Fatalf("matches = %d, teams = %d", len(matches), len(teams))
	}
	want := domain.Competition{Name: "ASEAN Championship", Path: "/football/asia/asean-championship/", Region: "Asia"}
	if matches[0].Competition != want {
		t.Errorf("competition = %+v", matches[0].Competition)
	}
	for _, tm := range teams {
		if tm.Slug == "" {
			t.Errorf("team %s has no slug", tm.Name)
		}
	}
}
