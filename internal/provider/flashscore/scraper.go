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

const baseURL = "https://www.flashscore.com"

// seasonPattern reads the season from the page heading, "2026/2027" or "2026".
// The <title> is not reliable: the Super League page says "Super League Indonesia".
var seasonPattern = regexp.MustCompile(`heading__info">(\d{4})(?:/\d{4})?<`)

// League pages quote the feed names with ' and team pages with ".
var feedPatterns = map[string]*regexp.Regexp{
	"fixtures": regexp.MustCompile("cjs\\.initialFeeds\\[['\"]fixtures['\"]\\]\\s*=\\s*\\{\\s*data:\\s*`(.*?)`"),
	"results":  regexp.MustCompile("cjs\\.initialFeeds\\[['\"]results['\"]\\]\\s*=\\s*\\{\\s*data:\\s*`(.*?)`"),
}

// ScrapeLeague reads fixtures (scheduled and live) and results of one competition
// page, e.g. path "/football/indonesia/super-league/", from one page load.
func ScrapeLeague(path string) ([]domain.Match, []domain.Team, error) {
	html, err := fetchPage(path)
	if err != nil {
		return nil, nil, err
	}

	found := seasonPattern.FindStringSubmatch(html)
	if len(found) < 2 {
		return nil, nil, fmt.Errorf("season not found in page heading")
	}
	season, _ := strconv.Atoi(found[1])

	matches, teams, err := parseFeeds(html)
	for i := range matches {
		matches[i].Season = season
	}
	return matches, teams, err
}

// ScrapeTeam reads a team's recent results and fixtures across all its
// competitions, e.g. path "/team/indonesia/88ErHiT9/results/". Each match
// carries its competition; the season is the year it is played (WIB).
// ponytail: a tournament spanning New Year (ASEAN Championship) splits over two seasons; read the season from the competition page if that matters.
func ScrapeTeam(path string) ([]domain.Match, []domain.Team, error) {
	html, err := fetchPage(path)
	if err != nil {
		return nil, nil, err
	}
	matches, teams, err := parseFeeds(html)
	for i := range matches {
		matches[i].Season = matches[i].MatchTime.In(domain.WIB).Year()
	}
	return matches, teams, err
}

func fetchPage(path string) (string, error) {
	req, err := http.NewRequest("GET", baseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// parseFeeds reads the fixtures and results feeds embedded in a page.
func parseFeeds(html string) ([]domain.Match, []domain.Team, error) {
	teamsMap := make(map[string]domain.Team)
	var matches []domain.Match

	for name, re := range feedPatterns {
		found := re.FindStringSubmatch(html)
		if len(found) < 2 {
			return nil, nil, fmt.Errorf("initialFeeds['%s'] not found", name)
		}

		phase, competition := "", domain.Competition{}
		for _, m := range parseRecords(found[1]) {
			// A "ZA" record opens a section, e.g. "INDONESIA: President Cup - Play Offs",
			// with the competition name (ZK), page (ZL), and region or country (ZY).
			if header, ok := m["ZA"]; ok {
				phase = sectionPhase(header)
				competition = domain.Competition{Name: m["ZK"], Path: m["ZL"], Region: m["ZY"]}
				continue
			}
			match, home, away, ok := parseMatch(m)
			if !ok {
				continue
			}
			match.Phase = phase
			match.Competition = competition
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
		Slug:         m["WU"],
		LogoURL:      imageURL(m["OA"]),
	}
	away := domain.Team{
		FlashscoreID: m["PY"],
		Name:         m["AF"],
		ShortName:    m["WN"],
		Slug:         m["WV"],
		LogoURL:      imageURL(m["OB"]),
	}

	ts, _ := strconv.ParseInt(m["AD"], 10, 64)
	homeScore, _ := strconv.Atoi(m["AG"])
	awayScore, _ := strconv.Atoi(m["AH"])
	stage, _ := strconv.Atoi(m["AC"])

	return domain.Match{
		FlashscoreID:         id,
		HomeTeamFlashscoreID: home.FlashscoreID,
		AwayTeamFlashscoreID: away.FlashscoreID,
		Status:               status(m["AB"]),
		MatchTime:            time.Unix(ts, 0),
		HomeScore:            homeScore,
		AwayScore:            awayScore,
		Stage:                stage,
		Round:                m["ER"],
		StageStartedAt:       unix(m["AO"]),
	}, home, away, true
}

// imageURL turns a Flashscore image id ("pbvGiliT-fudV7NWp.png") into its URL.
func imageURL(file string) string {
	if file == "" {
		return ""
	}
	return "https://static.flashscore.com/res/image/data/" + file
}

// maxImageSize caps a downloaded logo; real ones are a few KB.
const maxImageSize = 2 << 20

// DownloadImage fetches a PNG/JPEG/WebP image (team logo, player photo) and returns its bytes
// and file extension. Other content types are refused, so nothing scriptable
// (SVG, HTML) ends up served from our origin.
func DownloadImage(url string) ([]byte, string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	res, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("unexpected status %d", res.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(res.Body, maxImageSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(body) > maxImageSize {
		return nil, "", fmt.Errorf("image larger than %d bytes", maxImageSize)
	}

	ext, ok := map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}[http.DetectContentType(body)]
	if !ok {
		return nil, "", fmt.Errorf("not a PNG, JPEG, or WebP image")
	}
	return body, ext, nil
}

// sectionPhase returns what follows " - " in a section header, or "" for the main stage.
func sectionPhase(header string) string {
	if _, phase, ok := strings.Cut(header, " - "); ok {
		return phase
	}
	return ""
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

// SquadPlayer is one row of a team's squad page.
type SquadPlayer struct {
	FlashscoreID string
	Name         string
	Nationality  string
	ShirtNumber  int
	Position     string // GK, DF, MF, FW
}

var (
	squadSection = regexp.MustCompile(`lineupTable__title">([^<]*)<`)
	squadRow     = regexp.MustCompile(`(?s)lineupTable__cell--jersey">\s*(\d*)\s*</div>.*?(?:title="([^"]*)"></div>.*?)?href="/player/[^/]+/([A-Za-z0-9]+)/">\s*([^<]*?)\s*</a>`)
	squadRoles   = map[string]string{"Goalkeepers": "GK", "Defenders": "DF", "Midfielders": "MF", "Forwards": "FW"}
)

// ScrapeSquad reads a team's squad page, e.g. path "/team/persik-kediri/MDFOmQ7H/squad/".
func ScrapeSquad(path string) ([]SquadPlayer, error) {
	html, err := fetchPage(path)
	if err != nil {
		return nil, err
	}
	players := parseSquad(html)
	if len(players) == 0 {
		return nil, fmt.Errorf("no players on squad page")
	}
	return players, nil
}

// parseSquad reads the squad tables: a title (Goalkeepers, ..., Coach) then its
// rows. The page repeats the tables per competition; the first row of a player wins.
func parseSquad(html string) []SquadPlayer {
	titles := squadSection.FindAllStringSubmatchIndex(html, -1)
	seen := make(map[string]bool)
	var players []SquadPlayer
	for i, t := range titles {
		role, ok := squadRoles[html[t[2]:t[3]]]
		if !ok {
			continue // Coach
		}
		end := len(html)
		if i+1 < len(titles) {
			end = titles[i+1][0]
		}
		for _, r := range squadRow.FindAllStringSubmatch(html[t[1]:end], -1) {
			if seen[r[3]] {
				continue
			}
			seen[r[3]] = true
			number, _ := strconv.Atoi(r[1])
			players = append(players, SquadPlayer{FlashscoreID: r[3], Name: r[4], Nationality: r[2], ShirtNumber: number, Position: role})
		}
	}
	return players
}
