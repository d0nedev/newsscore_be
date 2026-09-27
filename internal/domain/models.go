package domain

import "time"

// WIB is the product's home time zone: a "date" is a calendar day in Indonesia.
var WIB = time.FixedZone("WIB", 7*60*60)

// DateLayout is the API contract's "DD.MM.YYYY" (docs/development-plan/api-contract-plan.md).
const DateLayout = "02.01.2006"

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
	Round                string    // e.g. "Round 3", "Final"
	Phase                string    // e.g. "Play Offs"; empty for the regular season or group stage
}
