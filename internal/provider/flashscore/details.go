package flashscore

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func fetchFeed(feedType, matchID string) ([]map[string]string, error) {
	// Add delay to prevent IP blocking
	time.Sleep(1 * time.Second)

	url := fmt.Sprintf("https://www.flashscore.com/x/feed/%s_%s", feedType, matchID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	req.Header.Set("x-fsign", "SW9D1eZo")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d", res.StatusCode)
	}

	body, _ := io.ReadAll(res.Body)
	records := strings.Split(string(body), "¬~")

	var out []map[string]string
	for _, r := range records {
		if strings.TrimSpace(r) == "" {
			continue
		}
		parts := strings.Split(r, "¬")
		m := make(map[string]string)
		for _, p := range parts {
			kv := strings.SplitN(p, "÷", 2)
			if len(kv) == 2 {
				m[kv[0]] = kv[1]
			}
		}
		if len(m) > 0 {
			out = append(out, m)
		}
	}
	return out, nil
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
	statsRec, err := fetchFeed("df_st_1", matchID)
	var stats []MatchStat
	if err == nil {
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
	eventsRec, err := fetchFeed("df_sui_1", matchID)
	var events []MatchEvent
	if err == nil {
		for _, m := range eventsRec {
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
	}

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

	return events, stats, players, nil
}
