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
	Stage                int       // Flashscore stage code (AC/DB): 12 1st half, 38 half time, 13 2nd half
	StageStartedAt       time.Time // live minute = now - StageStartedAt (+45 in the 2nd half)
	UpdatedAt            time.Time
}
