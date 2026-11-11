package flashscore

import (
	"encoding/json"
	"errors"
	"os"
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

func TestParseLineupGraphQL(t *testing.T) {
	// Trimmed from a real dlie2 response (Crystal Palace v Liverpool): two starters
	// and one substitute per side.
	raw, err := os.ReadFile("testdata/lineup.json")
	if err != nil {
		t.Fatal(err)
	}
	var r lineupResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	lineup := parseLineupGraphQL(r)
	if lineup.Formations != [2]string{"3-4-2-1", "4-2-3-1"} {
		t.Errorf("formations = %q, want 3-4-2-1 and 4-2-3-1", lineup.Formations)
	}
	photo := "https://static.flashscore.com/res/image/data/"
	got := lineup.Players
	want := []Player{
		{FlashscoreID: "OpFOJojt", Name: "Henderson D.", Nationality: "England", ShirtNumber: 1, Team: 1, Starter: true, Goalkeeper: true,
			FormationPlace: 1, Rating: "6.6", PhotoURL: photo + "f5Ze1Ekg-C4ducc4l.png"},
		{FlashscoreID: "fVGFlDh2", Name: "Richards C.", Nationality: "USA", ShirtNumber: 26, Team: 1, Starter: true, FormationPlace: 2, Rating: "5.9"},
		{FlashscoreID: "rVwIjuW2", Name: "Benitez W.", Nationality: "Argentina", ShirtNumber: 44, Team: 1, Goalkeeper: true},
		{FlashscoreID: "UZrQ3bwP", Name: "Alisson", Nationality: "Brazil", ShirtNumber: 1, Team: 2, Starter: true, Goalkeeper: true, FormationPlace: 1, Rating: "8.7"},
		{FlashscoreID: "M3sbJix9", Name: "Araujo R.", Nationality: "Uruguay", ShirtNumber: 33, Team: 2, Starter: true, FormationPlace: 2, Rating: "6.6"},
		{FlashscoreID: "r7gln7MG", Name: "Endo W.", Nationality: "Japan", ShirtNumber: 3, Team: 2},
	}
	if len(got) != len(want) {
		t.Fatalf("players = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if want[i].PhotoURL == "" {
			got[i].PhotoURL = "" // only the first player's photo is pinned
		}
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
	// AM is a match note; LaLiga's hold newlines.
	match := "AA÷0hBlRgoL¬AD÷1790341200¬AB÷3¬PX÷88ErHiT9¬AE÷Indonesia¬WU÷indonesia¬AG÷2¬PY÷G0dqs0hU¬AF÷Singapore¬WV÷singapore¬AH÷0¬AM÷Playing home matches at a different stadium\n- Estadio.¬~"
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

func TestParseSquad(t *testing.T) {
	row := func(number, country, id, name string) string {
		return `<div class="lineupTable__row"><div class="lineupTable__cell lineupTable__cell--jersey">` + number + `</div>` +
			`<div class="lineupTable__cell--flag flag fl_228" title="` + country + `"></div>` +
			`<a class="lineupTable__cell--name" href="/player/x/` + id + `/">` + name + `</a></div>`
	}
	table := `<div class="lineupTable__title">Goalkeepers</div>` + row("33", "Indonesia", "vy7eaNdG", "Al Malik Husna") +
		`<div class="lineupTable__title">Forwards</div>` + row("", "Brazil", "vRktJf5L", "Rendy") +
		`<div class="lineupTable__title">Coach</div>` + row("", "Indonesia", "c0achId1", "Coach")
	// The page repeats the tables per competition.
	players := parseSquad(table + table)

	if len(players) != 2 {
		t.Fatalf("players = %+v", players)
	}
	if p := players[0]; p.Position != "GK" || p.ShirtNumber != 33 || p.Nationality != "Indonesia" || p.Name != "Al Malik Husna" {
		t.Errorf("goalkeeper = %+v", p)
	}
	if p := players[1]; p.Position != "FW" || p.ShirtNumber != 0 || p.FlashscoreID != "vRktJf5L" {
		t.Errorf("forward = %+v", p)
	}
}

func TestSectionPhase(t *testing.T) {
	for header, want := range map[string]string{
		"INDONESIA: Super League":                  "",
		"INDONESIA: President Cup - Play Offs":     "Play Offs",
		"EUROPE: Champions League - League phase":  "",
		"EUROPE: Champions League - Qualification": "Qualification",
	} {
		if got := sectionPhase(header); got != want {
			t.Errorf("sectionPhase(%q) = %q, want %q", header, got, want)
		}
	}
}

func TestCheckStatus(t *testing.T) {
	if err := checkStatus(200); err != nil {
		t.Errorf("200: %v", err)
	}
	for _, code := range []int{403, 429} {
		if err := checkStatus(code); !errors.Is(err, ErrBlocked) {
			t.Errorf("%d: %v, want ErrBlocked", code, err)
		}
	}
	if err := checkStatus(404); !errors.Is(err, ErrNotFound) || errors.Is(err, ErrBlocked) {
		t.Errorf("404: %v, want ErrNotFound", err)
	}
	if err := checkStatus(500); err == nil || errors.Is(err, ErrBlocked) || errors.Is(err, ErrNotFound) {
		t.Errorf("500: %v, want a plain error", err)
	}
}

func TestSeasonOf(t *testing.T) {
	for _, c := range []struct {
		date  string
		split bool
		want  int
	}{
		{"2026-08-22", true, 2026}, // Premier League 2026/2027 kick-off
		{"2027-01-16", true, 2026}, // same season after New Year
		{"2027-05-23", true, 2026},
		{"2026-02-21", false, 2026}, // MLS 2026
		{"2025-10-12", false, 2025}, // MLS 2025, still on a team's results page
	} {
		d, _ := time.ParseInLocation("2006-01-02", c.date, time.UTC)
		if got := SeasonOf(d.Add(12*time.Hour), c.split); got != c.want {
			t.Errorf("SeasonOf(%s, split=%v) = %d, want %d", c.date, c.split, got, c.want)
		}
	}
}

func TestSeasonPatternSplit(t *testing.T) {
	for heading, split := range map[string]bool{`heading__info">2026/2027<`: true, `heading__info">2026<`: false} {
		m := seasonPattern.FindStringSubmatch(heading)
		if len(m) < 3 || m[1] != "2026" || (m[2] != "") != split {
			t.Errorf("%s: %q", heading, m)
		}
	}
}

func TestHeadingLogo(t *testing.T) {
	page := `<div class="heading"><img class="heading__logo heading__logo--1" src="https://static.flashscore.com/res/image/data/QuGrSijl-fXN2TGqm.png" alt="Persib Bandung"></div>`
	if got, want := headingLogo(page), "https://static.flashscore.com/res/image/data/QuGrSijl-fXN2TGqm.png"; got != want {
		t.Errorf("headingLogo = %q, want %q", got, want)
	}
	if got := headingLogo(`<img class="heading__logo" src="https://evil.example/x.png">`); got != "" {
		t.Errorf("headingLogo took a foreign host: %q", got)
	}
}

// A "show more" fixtures page (tf_ feed), trimmed: one section, one match.
const moreFeed = "SA÷1¬~ZA÷INDONESIA: Super League¬ZEE÷8SwKOBWT¬ZB÷228¬ZY÷Indonesia¬ZL÷/football/indonesia/super-league/¬~" +
	"AA÷fFAhBUV1¬AD÷1797595200¬AB÷1¬CR÷1¬ER÷Round 15¬WM÷BAL¬PX÷W82OaQl9¬AE÷Bali United¬WU÷bali-united¬OA÷Me4JWEme-K6ujqDls.png¬" +
	"WN÷PSI¬PY÷0tqnseis¬AF÷PSIM Yogyakarta¬WV÷psim-yogyakarta¬OB÷lCS3WI5k-IFY6Af8o.png¬AN÷n¬~"

func TestParseMoreFeed(t *testing.T) {
	teams := map[string]domain.Team{}
	got := parseFeed(moreFeed, teams)
	if len(got) != 1 || got[0].FlashscoreID != "fFAhBUV1" || got[0].Round != "Round 15" || got[0].Status != "scheduled" ||
		got[0].Competition.Path != "/football/indonesia/super-league/" {
		t.Fatalf("parseFeed = %+v", got)
	}
	if len(teams) != 2 {
		t.Errorf("teams = %d, want 2", len(teams))
	}

	page := `cjs.Api.config = { country_id = 228; tournament_id = "8SwKOBWT"; seasonId: 190 }`
	if c, tr, s := pageCountryPattern.FindStringSubmatch(page), pageTournamentPattern.FindStringSubmatch(page), pageSeasonPattern.FindStringSubmatch(page); c == nil || tr == nil || s == nil ||
		c[1] != "228" || tr[1] != "8SwKOBWT" || s[1] != "190" {
		t.Errorf("ids = %v %v %v", c, tr, s)
	}
}
