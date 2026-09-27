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

func ScrapeResults() ([]domain.Match, []domain.Team, error) {
	req, err := http.NewRequest("GET", "https://www.flashscore.com/football/indonesia/super-league/results/", nil)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")

	res, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer res.Body.Close()

	body, _ := io.ReadAll(res.Body)
	html := string(body)

	re := regexp.MustCompile("cjs\\.initialFeeds\\['results'\\]\\s*=\\s*\\{\\s*data:\\s*`(.*?)`")
	matchStr := re.FindStringSubmatch(html)
	if len(matchStr) < 2 {
		return nil, nil, fmt.Errorf("initialFeeds['results'] not found")
	}
	dataStr := matchStr[1]

	records := strings.Split(dataStr, "¬~")

	teamsMap := make(map[string]domain.Team)
	var matches []domain.Match

	for _, r := range records {
		parts := strings.Split(r, "¬")
		m := make(map[string]string)
		for _, p := range parts {
			kv := strings.SplitN(p, "÷", 2)
			if len(kv) == 2 {
				m[kv[0]] = kv[1]
			}
		}

		if matchIDStr, ok := m["AA"]; ok {
			// Extract Teams
			homeID := m["PX"]
			awayID := m["PY"]

			if _, exists := teamsMap[homeID]; !exists {
				teamsMap[homeID] = domain.Team{
					FlashscoreID: homeID,
					Name:         m["AE"],
					ShortName:    m["WM"],
					LogoURL:      "https://static.flashscore.com/res/image/data/" + m["OA"],
				}
			}
			if _, exists := teamsMap[awayID]; !exists {
				teamsMap[awayID] = domain.Team{
					FlashscoreID: awayID,
					Name:         m["AF"],
					ShortName:    m["WN"],
					LogoURL:      "https://static.flashscore.com/res/image/data/" + m["OB"],
				}
			}

			// Extract Match
			ts, _ := strconv.ParseInt(m["AD"], 10, 64)
			homeScore, _ := strconv.Atoi(m["AG"])
			awayScore, _ := strconv.Atoi(m["AH"])

			status := "finished"
			if m["AB"] != "3" { // 3 means finished in flashscore
				status = "scheduled"
			}

			matches = append(matches, domain.Match{
				FlashscoreID:         matchIDStr,
				Season:               2026, // hardcoded for MVP
				HomeTeamFlashscoreID: homeID,
				AwayTeamFlashscoreID: awayID,
				Status:               status,
				MatchTime:            time.Unix(ts, 0),
				HomeScore:            homeScore,
				AwayScore:            awayScore,
				UpdatedAt:            time.Now(),
			})
		}
	}

	var teams []domain.Team
	for _, t := range teamsMap {
		teams = append(teams, t)
	}

	return matches, teams, nil
}
