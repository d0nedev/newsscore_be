package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
)

func main() {
	res, err := http.Get("https://en.wikipedia.org/wiki/2024%E2%80%9325_Liga_1_(Indonesia)")
	if err != nil {
		panic(err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	html := string(body)

	// find the standings table
	tableStart := strings.Index(html, "<th scope=\"col\">Pos")
	if tableStart == -1 {
		panic("Table not found")
	}
	html = html[tableStart:]
	tableEnd := strings.Index(html, "</tbody>")
	html = html[:tableEnd]

	// Find all rows
	rowRe := regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	rows := rowRe.FindAllStringSubmatch(html, -1)

	type Standing struct {
		Rank   string `json:"rank"`
		Team   string `json:"team"`
		Played string `json:"played"`
		Pts    string `json:"points"`
	}
	
	var standings []Standing

	cellRe := regexp.MustCompile(`(?s)<t[hd][^>]*>(.*?)</t[hd]>`)
	for _, row := range rows {
		cells := cellRe.FindAllStringSubmatch(row[1], -1)
		if len(cells) >= 10 {
			rank := stripTags(cells[0][1])
			team := stripTags(cells[1][1])
			played := stripTags(cells[2][1])
			pts := stripTags(cells[9][1])

			// clean up strings
			rank = strings.TrimSpace(rank)
			team = strings.Split(team, "[")[0] // remove wikipedia citations like [a]
			team = strings.TrimSpace(team)
			played = strings.TrimSpace(played)
			pts = strings.TrimSpace(pts)
			if rank != "" && rank != "Pos" {
				standings = append(standings, Standing{
					Rank:   rank,
					Team:   team,
					Played: played,
					Pts:    pts,
				})
			}
		}
	}

	out, _ := json.MarshalIndent(map[string]interface{}{
		"league":    "Liga 1 Indonesia 2024-25",
		"standings": standings,
	}, "", "  ")
	os.WriteFile("liga1_standings_sample.json", out, 0644)
	fmt.Println("Scraped", len(standings), "teams and saved to liga1_standings_sample.json")
}

func stripTags(s string) string {
	re := regexp.MustCompile(`<[^>]*>`)
	s = re.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\n", "")
	return s
}
