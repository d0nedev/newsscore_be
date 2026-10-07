// Package sofascore reads SofaScore's JSON API (www.sofascore.com/api/v1) for
// competitions Flashscore lacks. The API answers 403 to Go's own TLS, so
// requests go through a Chrome TLS fingerprint (tls-client) with Chrome's headers.
package sofascore

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	nethttp "net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	http "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

const base = "https://www.sofascore.com/api/v1"

// ErrBlocked means SofaScore refused us (403 or 429): back off, do not retry at once.
var ErrBlocked = errors.New("blocked by sofascore")

// ErrNotFound is SofaScore's 404: the data does not exist (no lineups, no stats, no next page).
var ErrNotFound = errors.New("not found on sofascore")

// The fingerprint must match the headers' Chrome version: Chrome_150 with these headers gets 403.
var client = sync.OnceValues(func() (tls_client.HttpClient, error) {
	return tls_client.NewHttpClient(tls_client.NewNoopLogger(),
		tls_client.WithClientProfile(profiles.Chrome_152), tls_client.WithTimeoutSeconds(30))
})

// proxy is the base URL of scrap-claude/sofascore_proxy.py; see SetProxy.
var (
	proxy       string
	proxyClient = &nethttp.Client{Timeout: time.Minute} // the browser may reload for a fresh token first
)

// SetProxy sends every request through a browser that passes SofaScore's
// captcha (scrap-claude/sofascore_proxy.py), e.g. "http://localhost:8787".
// SofaScore asks for one from flagged IPs (VPNs, heavy users); empty calls it directly.
func SetProxy(url string) { proxy = strings.TrimSuffix(url, "/") }

func get(path string, v any) error {
	time.Sleep(time.Second) // same politeness gap as the Flashscore feeds
	if proxy != "" {
		res, err := proxyClient.Get(proxy + "/api/v1" + path)
		if err != nil {
			return err
		}
		defer res.Body.Close()
		return decode(path, res.StatusCode, res.Body, v)
	}
	res, err := fetch(base + path)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	return decode(path, res.StatusCode, res.Body, v)
}

// fetch GETs url with Chrome's TLS fingerprint and headers.
func fetch(url string) (*http.Response, error) {
	c, err := client()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header = http.Header{
		"sec-ch-ua":          {`"Not;A=Brand";v="8", "Chromium";v="152", "Google Chrome";v="152"`},
		"sec-ch-ua-mobile":   {"?0"},
		"sec-ch-ua-platform": {`"macOS"`},
		"user-agent":         {"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/152.0.0.0 Safari/537.36"},
		"accept":             {"*/*"},
		"accept-encoding":    {"gzip, deflate, br, zstd"},
		"accept-language":    {"en-US,en;q=0.9"},
		http.HeaderOrderKey:  {"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "user-agent", "accept", "accept-encoding", "accept-language"},
	}
	return c.Do(req)
}

const maxImageSize = 2 << 20 // same cap as Flashscore logos; real ones are a few KB

// DownloadImage fetches a logo or photo from img.sofascore.com, which has no
// captcha but answers 403 to Go's own TLS. A 404 (no picture) is ErrNotFound.
func DownloadImage(url string) ([]byte, string, error) {
	res, err := fetch(url)
	if err != nil {
		return nil, "", err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, "", fmt.Errorf("%w: %s", ErrNotFound, url)
	default:
		return nil, "", fmt.Errorf("sofascore image %s: status %d", url, res.StatusCode)
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

func decode(path string, status int, r io.Reader, v any) error {
	switch {
	case status == http.StatusNotFound:
		return fmt.Errorf("%w: %s", ErrNotFound, path)
	case status == http.StatusForbidden || status == http.StatusTooManyRequests:
		return fmt.Errorf("%w: status %d", ErrBlocked, status)
	case status != http.StatusOK:
		return fmt.Errorf("sofascore %s: status %d", path, status)
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("sofascore %s: %w", path, err)
	}
	return nil
}

type Team struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
	NameCode  string `json:"nameCode"`
}

// Short is the name for tight spaces: shortName, else the 3-letter code, else the name.
func (t Team) Short() string {
	for _, s := range []string{t.ShortName, t.NameCode} {
		if s != "" {
			return s
		}
	}
	return t.Name
}

type Player struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Position     string `json:"position"` // G, D, M, F
	JerseyNumber string `json:"jerseyNumber"`
	Height       int    `json:"height"`
	DateOfBirth  int64  `json:"dateOfBirthTimestamp"`
	Country      struct {
		Name string `json:"name"`
	} `json:"country"`
}

var positions = map[string]string{"G": "GK", "D": "DF", "M": "MF", "F": "FW"}

// Pos maps SofaScore's position letter to ours (GK, DF, MF, FW); "" when unknown.
func (p Player) Pos() string { return positions[p.Position] }

// Born is the date of birth, zero when unknown.
func (p Player) Born() time.Time {
	if p.DateOfBirth == 0 {
		return time.Time{}
	}
	return time.Unix(p.DateOfBirth, 0).UTC()
}

// Season is a competition's latest season: its id and first year ("25/26" -> 2025, "2026" -> 2026).
type Season struct {
	ID   int
	Year int16
}

func LatestSeason(tournament int) (Season, error) {
	var out struct {
		Seasons []struct {
			ID   int    `json:"id"`
			Year string `json:"year"`
		} `json:"seasons"`
	}
	if err := get(fmt.Sprintf("/unique-tournament/%d/seasons", tournament), &out); err != nil {
		return Season{}, err
	}
	if len(out.Seasons) == 0 {
		return Season{}, fmt.Errorf("tournament %d has no seasons", tournament)
	}
	return Season{ID: out.Seasons[0].ID, Year: seasonYear(out.Seasons[0].Year)}, nil
}

func seasonYear(y string) int16 {
	first, _, _ := strings.Cut(y, "/")
	n, _ := strconv.Atoi(first)
	if n < 100 {
		n += 2000
	}
	return int16(n)
}

type Event struct {
	ID     int `json:"id"`
	Status struct {
		Type string `json:"type"` // notstarted, inprogress, finished, postponed, canceled, ...
	} `json:"status"`
	StartTimestamp int64 `json:"startTimestamp"`
	RoundInfo      struct {
		Round int    `json:"round"`
		Name  string `json:"name"` // knockout rounds only: "Final", "Semifinals"
	} `json:"roundInfo"`
	HomeTeam  Team  `json:"homeTeam"`
	AwayTeam  Team  `json:"awayTeam"`
	HomeScore Score `json:"homeScore"`
	AwayScore Score `json:"awayScore"`
}

type Score struct {
	Current *int `json:"current"`
}

// MatchStatus maps SofaScore's status to ours: scheduled, live, or finished.
// ponytail: postponed and canceled read as scheduled, like Flashscore's mapping.
func (e Event) MatchStatus() string {
	switch e.Status.Type {
	case "inprogress":
		return "live"
	case "finished":
		return "finished"
	}
	return "scheduled"
}

// Round is the knockout round's name, else "Round N"; "" when SofaScore gives neither.
func (e Event) Round() string {
	switch {
	case e.RoundInfo.Name != "":
		return e.RoundInfo.Name
	case e.RoundInfo.Round > 0:
		return "Round " + strconv.Itoa(e.RoundInfo.Round)
	}
	return ""
}

// Events lists a season's matches: every page of results, then every page of fixtures.
func Events(tournament, season int) ([]Event, error) {
	var all []Event
	for _, kind := range []string{"last", "next"} {
		for page := 0; ; page++ {
			var out struct {
				Events      []Event `json:"events"`
				HasNextPage bool    `json:"hasNextPage"`
			}
			err := get(fmt.Sprintf("/unique-tournament/%d/season/%d/events/%s/%d", tournament, season, kind, page), &out)
			if errors.Is(err, ErrNotFound) {
				break // no (more) matches of this kind
			}
			if err != nil {
				return nil, err
			}
			all = append(all, out.Events...)
			if !out.HasNextPage {
				break
			}
		}
	}
	return all, nil
}

type Table struct {
	Name string `json:"name"` // "Liga Topskor U17, Group A 2026"
	Rows []struct {
		Position      int  `json:"position"`
		Matches       int  `json:"matches"`
		Wins          int  `json:"wins"`
		Draws         int  `json:"draws"`
		Losses        int  `json:"losses"`
		ScoresFor     int  `json:"scoresFor"`
		ScoresAgainst int  `json:"scoresAgainst"`
		Points        int  `json:"points"`
		Team          Team `json:"team"`
	} `json:"rows"`
}

func Standings(tournament, season int) ([]Table, error) {
	var out struct {
		Standings []Table `json:"standings"`
	}
	err := get(fmt.Sprintf("/unique-tournament/%d/season/%d/standings/total", tournament, season), &out)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return out.Standings, err
}

type TopPlayer struct {
	Player     Player                     `json:"player"`
	Team       Team                       `json:"team"`
	Statistics map[string]json.RawMessage `json:"statistics"` // the category's value, appearances, ...
}

// TopPlayers is each leaderboard by SofaScore's category key (goals, assists, rating, ...), best first.
func TopPlayers(tournament, season int) (map[string][]TopPlayer, error) {
	var out struct {
		TopPlayers map[string][]TopPlayer `json:"topPlayers"`
	}
	err := get(fmt.Sprintf("/unique-tournament/%d/season/%d/top-players/overall", tournament, season), &out)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return out.TopPlayers, err
}

// Squad is a team's current players.
func Squad(team int) ([]Player, error) {
	var out struct {
		Players []struct {
			Player Player `json:"player"`
		} `json:"players"`
	}
	if err := get(fmt.Sprintf("/team/%d/players", team), &out); err != nil {
		return nil, err
	}
	players := make([]Player, 0, len(out.Players))
	for _, p := range out.Players {
		players = append(players, p.Player)
	}
	return players, nil
}

type Incident struct {
	ID        int     `json:"id"`
	Type      string  `json:"incidentType"`  // goal, card, substitution, inGamePenalty, period, ...
	Class     string  `json:"incidentClass"` // regular, penalty, ownGoal, yellow, red, yellowRed, missed, ...
	Time      int     `json:"time"`
	AddedTime int     `json:"addedTime"`
	IsHome    bool    `json:"isHome"`
	Player    *Player `json:"player"`
	PlayerIn  *Player `json:"playerIn"`
	PlayerOut *Player `json:"playerOut"`
	Assist    *Player `json:"assist1"`
	// Free-text names, set when the editor typed a player SofaScore does not know.
	PlayerName    string `json:"playerName"`
	PlayerNameIn  string `json:"playerNameIn"`
	PlayerNameOut string `json:"playerNameOut"`
}

// MatchEvent is an incident in the shape Flashscore's are stored: type names
// match theirs ("Goal", "Yellow Card", ...) so the same queries count both.
type MatchEvent struct {
	ID      string
	Type    string
	Minute  string // "38'", "90+3'"
	Home    bool
	Player  Ref
	Related Ref // the assist on a goal, the player going off on a substitution
}

// Ref is a player as an incident names them; ID is 0 when SofaScore has no player.
type Ref struct {
	ID   int
	Name string
}

func ref(p *Player, name string) Ref {
	if p != nil {
		return Ref{ID: p.ID, Name: p.Name}
	}
	return Ref{Name: name}
}

var eventTypes = map[string]string{
	"goal/regular":         "Goal",
	"goal/penalty":         "Penalty",
	"goal/ownGoal":         "Own goal",
	"card/yellow":          "Yellow Card",
	"card/red":             "Red Card",
	"card/yellowRed":       "Yellow/Red Card",
	"inGamePenalty/missed": "Penalty missed",
}

// Events keeps goals, cards, missed penalties, and substitutions; periods,
// added time, and VAR checks are dropped.
func (i Incident) Event() (MatchEvent, bool) {
	e := MatchEvent{ID: strconv.Itoa(i.ID), Home: i.IsHome, Minute: strconv.Itoa(i.Time) + "'"}
	if i.AddedTime > 0 && i.AddedTime != 999 { // 999 marks a period's end
		e.Minute = fmt.Sprintf("%d+%d'", i.Time, i.AddedTime)
	}
	if i.Type == "substitution" {
		e.Type, e.Player, e.Related = "Substitution", ref(i.PlayerIn, i.PlayerNameIn), ref(i.PlayerOut, i.PlayerNameOut)
		return e, true
	}
	t, ok := eventTypes[i.Type+"/"+i.Class]
	if !ok {
		return MatchEvent{}, false
	}
	e.Type, e.Player, e.Related = t, ref(i.Player, i.PlayerName), ref(i.Assist, "")
	return e, true
}

type LineupPlayer struct {
	Player      Player          `json:"player"`
	ShirtNumber int             `json:"shirtNumber"`
	Substitute  bool            `json:"substitute"`
	Statistics  json.RawMessage `json:"statistics"`
}

type Stat struct {
	Name string `json:"name"`
	Home string `json:"home"`
	Away string `json:"away"`
}

// Details is what a finished match's page shows: incidents, both lineups, and team statistics.
type Details struct {
	Incidents []Incident
	Home      []LineupPlayer
	Away      []LineupPlayer
	Stats     []Stat // whole match; empty when SofaScore has none
}

// MatchDetails reads a match's incidents, lineups, and statistics. Lineups and
// statistics are often missing for editor-run competitions (404): those stay empty.
func MatchDetails(event int) (Details, error) {
	var d Details
	var inc struct {
		Incidents []Incident `json:"incidents"`
	}
	if err := get(fmt.Sprintf("/event/%d/incidents", event), &inc); err != nil && !errors.Is(err, ErrNotFound) {
		return d, err
	}
	d.Incidents = inc.Incidents

	var lineups struct {
		Home struct {
			Players []LineupPlayer `json:"players"`
		} `json:"home"`
		Away struct {
			Players []LineupPlayer `json:"players"`
		} `json:"away"`
	}
	if err := get(fmt.Sprintf("/event/%d/lineups", event), &lineups); err != nil && !errors.Is(err, ErrNotFound) {
		return d, err
	}
	d.Home, d.Away = lineups.Home.Players, lineups.Away.Players

	var stats struct {
		Statistics []struct {
			Period string `json:"period"`
			Groups []struct {
				Items []Stat `json:"statisticsItems"`
			} `json:"groups"`
		} `json:"statistics"`
	}
	if err := get(fmt.Sprintf("/event/%d/statistics", event), &stats); err != nil && !errors.Is(err, ErrNotFound) {
		return d, err
	}
	for _, p := range stats.Statistics {
		if p.Period != "ALL" {
			continue
		}
		for _, g := range p.Groups {
			d.Stats = append(d.Stats, g.Items...)
		}
	}
	return d, nil
}
