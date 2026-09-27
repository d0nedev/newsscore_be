package domain

import "time"

type Team struct {
	FlashscoreID string
	Name         string
	ShortName    string
	LogoURL      string
}

type Match struct {
	FlashscoreID         string
	Season               int
	HomeTeamFlashscoreID string
	AwayTeamFlashscoreID string
	Status               string
	MatchTime            time.Time
	HomeScore            int
	AwayScore            int
	UpdatedAt            time.Time
}

type Standing struct {
	Season  int
	TeamID  string
	Rank    int
	Points  int
	Form    string
	Zone    string
	Matches int
	Wins    int
	Draws   int
	Losses  int
	GF      int
	GA      int
}
