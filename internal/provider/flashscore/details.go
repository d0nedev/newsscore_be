package flashscore

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
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
	// Add delay to prevent IP blocking
	time.Sleep(1 * time.Second)

	url := fmt.Sprintf("https://www.flashscore.com/x/feed/%s_%s", feedType, matchID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("x-fsign", fsign)

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	return parseRecords(string(body)), nil
}

type MatchEvent struct {
	ID         string
	PlayerName string
	PlayerID   string
	Minute     string
	Type       string // e.g., "Goal", "Yellow Card"
	Team       int    // 1 (Home) or 2 (Away)
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
	ShirtNumber  string
	Team         int // 1 or 2
}

func ScrapeMatchDetails(matchID string) ([]MatchEvent, []MatchStat, []Player, error) {
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

	// 3. Lineups (Players)
	lineupsRec, err := fetchFeed("df_li_1", matchID)
	var players []Player
	if err == nil {
		currentTeam := 1 // Home usually first
		for _, m := range lineupsRec {
			if m["LC"] == "1" {
				currentTeam = 1
			} else if m["LC"] == "2" {
				currentTeam = 2
			}

			if pid, ok := m["LP"]; ok {
				players = append(players, Player{
					FlashscoreID: pid,
					Name:         m["LI"],
					Nationality:  m["LQ"],
					ShirtNumber:  m["LJ"],
					Team:         currentTeam,
				})
			}
		}
	}

	// Only fail when nothing usable came back; lineups alone are not worth saving.
	if statsErr != nil && eventsErr != nil {
		return nil, nil, nil, fmt.Errorf("stats: %w; events: %w", statsErr, eventsErr)
	}

	return events, stats, players, nil
}

// ScrapeEvents reads the match incidents feed (goals, cards, substitutions).
func ScrapeEvents(matchID string) ([]MatchEvent, error) {
	records, err := fetchFeed("df_sui_1", matchID)
	if err != nil {
		return nil, err
	}

	var events []MatchEvent
	for _, m := range records {
		if id, ok := m["III"]; ok {
			team := 1
			if m["IA"] == "2" {
				team = 2
			}
			events = append(events, MatchEvent{
				ID:         id,
				PlayerName: m["IF"],
				PlayerID:   m["IM"],
				Minute:     m["IB"],
				Type:       m["IK"],
				Team:       team,
			})
		}
	}
	return events, nil
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
