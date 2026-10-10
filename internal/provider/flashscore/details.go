package flashscore

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// fsign is the x-fsign header Flashscore's feeds require; it changes now and then (see docs/files/07-scraping-flashscore.md).
var fsign = "SW9D1eZo"

// SetFSign overrides the x-fsign header, e.g. from FLASHSCORE_FSIGN.
func SetFSign(v string) {
	if v != "" {
		fsign = v
	}
}

func fetchFeed(feedType, matchID string) ([]map[string]string, error) {
	body, err := fetchBody(feedType, matchID)
	if err != nil {
		return nil, err
	}
	return parseRecords(body), nil
}

func fetchBody(feedType, matchID string) (string, error) {
	// Add delay to prevent IP blocking
	time.Sleep(1 * time.Second)

	url := fmt.Sprintf("https://www.flashscore.com/x/feed/%s_%s", feedType, matchID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("x-fsign", fsign)

	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()

	if err := checkStatus(res.StatusCode); err != nil {
		return "", err
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

type MatchEvent struct {
	ID         string
	PlayerName string
	PlayerID   string
	Minute     string
	Type       string // e.g. "Goal", "Penalty", "Yellow Card", "Substitution"
	Team       int    // 1 (Home) or 2 (Away)
	// Related is the assist on a goal or the player leaving on a substitution.
	RelatedName string
	RelatedID   string
}

type MatchStat struct {
	Name string
	Home string
	Away string
}

type Player struct {
	FlashscoreID string
	Name         string
	Nationality  string
	ShirtNumber  int
	Team         int  // 1 or 2
	Starter      bool // false for substitutes
	PhotoURL     string
	Goalkeeper   bool
	// FormationPlace is a starter's slot, 1 (goalkeeper) to 11 in pitch order; 0 when not given.
	FormationPlace int
	Rating         string // Flashscore's player rating, e.g. "7.1"; "" before or without one
}

// Lineup is both teams' players and formations ("4-4-2"; "" when the feed has none).
type Lineup struct {
	Players    []Player
	Formations [2]string // home, away
}

func ScrapeMatchDetails(matchID string) ([]MatchEvent, []MatchStat, Lineup, error) {
	// 1. Stats
	statsRec, statsErr := fetchFeed("df_st_1", matchID)
	var stats []MatchStat
	if statsErr == nil {
		for _, m := range statsRec {
			if name, ok := m["SG"]; ok {
				stats = append(stats, MatchStat{
					Name: name,
					Home: m["SH"],
					Away: m["SI"],
				})
			}
		}
	}

	// 2. Events
	events, eventsErr := ScrapeEvents(matchID)

	// 3. Lineups
	lineup, _ := ScrapeLineup(matchID)

	// Only fail when nothing usable came back; lineups alone are not worth saving.
	if statsErr != nil && eventsErr != nil {
		return nil, nil, Lineup{}, fmt.Errorf("stats: %w; events: %w", statsErr, eventsErr)
	}

	return events, stats, lineup, nil
}

// ScrapeLineup reads only the lineups feed: players and formations, one request.
func ScrapeLineup(matchID string) (Lineup, error) {
	body, err := fetchBody("df_li_1", matchID)
	if err != nil {
		return Lineup{}, err
	}
	return parseLineups(body), nil
}

// ScrapeEvents reads the match incidents feed (goals, cards, substitutions).
func ScrapeEvents(matchID string) ([]MatchEvent, error) {
	body, err := fetchBody("df_sui_1", matchID)
	if err != nil {
		return nil, err
	}
	return parseIncidents(body), nil
}

// parseIncidents reads df_sui records. One incident can hold several parts, each
// starting at an "IE" key: a goal and its assist, a substitution's out and in, or
// "Penalty Awarded" then the penalty's outcome. A key-value map would keep only
// the last part, so fields are read in order.
func parseIncidents(body string) []MatchEvent {
	type part struct{ kind, name, id string }

	var events []MatchEvent
	for _, record := range strings.Split(body, "¬~") {
		var e MatchEvent
		var parts []part
		for _, field := range strings.Split(record, "¬") {
			k, v, ok := strings.Cut(field, "÷")
			if !ok {
				continue
			}
			switch k {
			case "III":
				e.ID = v
			case "IA":
				e.Team = 1
				if v == "2" {
					e.Team = 2
				}
			case "IB":
				e.Minute = v
			case "IE":
				parts = append(parts, part{})
			case "IK", "IF", "IM":
				if len(parts) == 0 {
					parts = append(parts, part{})
				}
				p := &parts[len(parts)-1]
				switch k {
				case "IK":
					p.kind = v
				case "IF":
					p.name = v
				case "IM":
					p.id = v
				}
			}
		}
		if e.ID == "" || len(parts) == 0 {
			continue
		}

		main, related := parts[0], part{}
		if len(parts) > 1 {
			related = parts[1]
		}
		switch {
		case main.kind == "Penalty Awarded" && len(parts) > 1:
			// The second part is what happened to the penalty; the awarding is noise.
			main, related = parts[1], part{}
		case main.kind == "Substitution - Out" && len(parts) > 1:
			// Report the player coming on, with the one going off as related.
			main, related = parts[1], parts[0]
			main.kind = "Substitution"
		}

		e.Type, e.PlayerName, e.PlayerID = main.kind, main.name, main.id
		e.RelatedName, e.RelatedID = related.name, related.id
		events = append(events, e)
	}
	return events
}

// parseLineups reads df_li records: "LB" opens a section (Starting Lineups,
// Substitutes, Coaches), "LC" switches team side, "LP" is a player, and "LD"
// the side's formation with the goalkeeper first ("1-4-4-2").
func parseLineups(body string) Lineup {
	var out Lineup
	section, side := "", 1
	for _, m := range parseRecords(body) {
		if v, ok := m["LB"]; ok {
			section = v
		}
		if v, ok := m["LC"]; ok {
			side = 1
			if v == "2" {
				side = 2
			}
		}
		if v, ok := m["LD"]; ok {
			out.Formations[side-1] = strings.TrimPrefix(v, "1-")
		}
		id, ok := m["LP"]
		if !ok || (section != "Starting Lineups" && section != "Substitutes") {
			continue
		}
		number, _ := strconv.Atoi(m["LJ"])
		place, _ := strconv.Atoi(m["LL"])
		out.Players = append(out.Players, Player{
			FormationPlace: place,
			Rating:         m["LPR"],
			FlashscoreID:   id,
			Name:           m["LI"],
			Nationality:    m["LQ"],
			ShirtNumber:    number,
			Team:           side,
			Starter:        section == "Starting Lineups",
			Goalkeeper:     m["LS"] == "Goalkeeper",
			PhotoURL:       imageURL(m["LPX"]), // 108x108; LPI and LPL are smaller cuts
		})
	}
	return out
}

// LiveState is the small "dc_1" core feed: status, stage, and score of one match.
type LiveState struct {
	Status         string
	Stage          int
	StageStartedAt time.Time
	HomeScore      int
	AwayScore      int
}

func ScrapeLive(matchID string) (LiveState, error) {
	records, err := fetchFeed("dc_1", matchID)
	if err != nil {
		return LiveState{}, err
	}
	if len(records) == 0 || records[0]["DA"] == "" {
		return LiveState{}, fmt.Errorf("empty live feed for %s", matchID)
	}

	return parseLive(records[0]), nil
}

func parseLive(m map[string]string) LiveState {
	stage, _ := strconv.Atoi(m["DB"])
	home, _ := strconv.Atoi(m["DE"])
	away, _ := strconv.Atoi(m["DF"])

	return LiveState{
		Status:         status(m["DA"]),
		Stage:          stage,
		StageStartedAt: unix(m["DD"]),
		HomeScore:      home,
		AwayScore:      away,
	}
}
