package flashscore

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/d0nedev/newsscore/internal/domain"
)

// client bounds every Flashscore call so a hung connection cannot stall the ingestor.
var client = &http.Client{Timeout: 30 * time.Second}

const leagueURL = "https://www.flashscore.com/football/indonesia/super-league/"

var feedPatterns = map[string]*regexp.Regexp{
	"fixtures": regexp.MustCompile("cjs\\.initialFeeds\\['fixtures'\\]\\s*=\\s*\\{\\s*data:\\s*`(.*?)`"),
	"results":  regexp.MustCompile("cjs\\.initialFeeds\\['results'\\]\\s*=\\s*\\{\\s*data:\\s*`(.*?)`"),
}

// ScrapeLeague reads fixtures (scheduled and live) and results from one page load.
func ScrapeLeague() ([]domain.Match, []domain.Team, error) {
	req, err := http.NewRequest("GET", leagueURL, nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	res, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("unexpected status %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, nil, err
	}
	html := string(body)

	teamsMap := make(map[string]domain.Team)
	var matches []domain.Match

	for name, re := range feedPatterns {
		found := re.FindStringSubmatch(html)
		if len(found) < 2 {
			return nil, nil, fmt.Errorf("initialFeeds['%s'] not found", name)
		}

		for _, m := range parseRecords(found[1]) {
			match, home, away, ok := parseMatch(m)
			if !ok {
				continue
			}
			teamsMap[home.FlashscoreID] = home
			teamsMap[away.FlashscoreID] = away
			matches = append(matches, match)
		}
	}

	teams := make([]domain.Team, 0, len(teamsMap))
	for _, t := range teamsMap {
		teams = append(teams, t)
	}

	return matches, teams, nil
}

func parseMatch(m map[string]string) (domain.Match, domain.Team, domain.Team, bool) {
	id, ok := m["AA"]
	if !ok {
		return domain.Match{}, domain.Team{}, domain.Team{}, false
	}

	home := domain.Team{
		FlashscoreID: m["PX"],
		Name:         m["AE"],
		ShortName:    m["WM"],
		LogoURL:      "https://static.flashscore.com/res/image/data/" + m["OA"],
	}
	away := domain.Team{
		FlashscoreID: m["PY"],
		Name:         m["AF"],
		ShortName:    m["WN"],
		LogoURL:      "https://static.flashscore.com/res/image/data/" + m["OB"],
	}

	ts, _ := strconv.ParseInt(m["AD"], 10, 64)
	homeScore, _ := strconv.Atoi(m["AG"])
	awayScore, _ := strconv.Atoi(m["AH"])
	stage, _ := strconv.Atoi(m["AC"])

	return domain.Match{
		FlashscoreID:         id,
		Season:               2026, // hardcoded for MVP
		HomeTeamFlashscoreID: home.FlashscoreID,
		AwayTeamFlashscoreID: away.FlashscoreID,
		Status:               status(m["AB"]),
		MatchTime:            time.Unix(ts, 0),
		HomeScore:            homeScore,
		AwayScore:            awayScore,
		Stage:                stage,
		StageStartedAt:       unix(m["AO"]),
		UpdatedAt:            time.Now(),
	}, home, away, true
}

// status maps Flashscore's AB/DA code: 1 scheduled, 2 live, 3 finished.
func status(code string) string {
	switch code {
	case "2":
		return "live"
	case "3":
		return "finished"
	}
	return "scheduled"
}

func unix(v string) time.Time {
	ts, err := strconv.ParseInt(v, 10, 64)
	if err != nil || ts == 0 {
		return time.Time{}
	}
	return time.Unix(ts, 0)
}

// parseRecords splits Flashscore's flat format: records by "¬~", fields by "¬", key/value by "÷".
func parseRecords(data string) []map[string]string {
	var out []map[string]string
	for _, r := range strings.Split(data, "¬~") {
		m := make(map[string]string)
		for _, p := range strings.Split(r, "¬") {
			if k, v, ok := strings.Cut(p, "÷"); ok {
				m[k] = v
			}
		}
		if len(m) > 0 {
			out = append(out, m)
		}
	}
	return out
}
