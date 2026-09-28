package main

import "testing"

func TestSeasonAndGroup(t *testing.T) {
	for year, want := range map[string]int16{"25/26": 2025, "2026": 2026, "": 0} {
		p := tournamentPage{}
		if year != "" {
			p.Seasons = append(p.Seasons, struct {
				Year string `json:"year"`
			}{year})
		}
		if got := p.season(); got != want {
			t.Errorf("season(%q) = %d, want %d", year, got, want)
		}
	}
	for name, want := range map[string]string{
		"Indonesia Nusantara League, Group A":      "Group A",
		"Liga Topskor U17, Second Round, Group A ": "Second Round, Group A",
		"3rd Place ":       "3rd Place",
		"Liga Topskor U17": "",
	} {
		competition := "Liga Topskor U17"
		if name == "Indonesia Nusantara League, Group A" {
			competition = "Indonesia Nusantara League"
		}
		if got := groupName(name, competition); got != want {
			t.Errorf("groupName(%q) = %q, want %q", name, got, want)
		}
	}
}
