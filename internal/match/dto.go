package match

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
	"uuid"

	"github.com/d0nedev/newsscore/internal/domain"
	"github.com/d0nedev/newsscore/internal/platform/apperror"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

var validStatuses = map[string]bool{"scheduled": true, "live": true, "finished": true}

type listFilter struct {
	From   time.Time
	TeamID *uuid.UUID
	Status string
	League string // competition slug
}

func parseListFilter(q url.Values) (listFilter, error) {
	var f listFilter

	date := q.Get("date")
	if date == "" {
		now := time.Now().In(domain.WIB)
		f.From = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, domain.WIB)
	} else {
		from, err := time.ParseInLocation(domain.DateLayout, date, domain.WIB)
		if err != nil {
			return f, apperror.Validation("date must be DD.MM.YYYY")
		}
		f.From = from
	}

	if v := q.Get("teamId"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return f, apperror.Validation("invalid teamId")
		}
		f.TeamID = &id
	}

	f.League = q.Get("leagueId")

	if v := q.Get("status"); v != "" {
		if !validStatuses[v] {
			return f, apperror.Validation("status must be scheduled, live, or finished")
		}
		f.Status = v
	}

	return f, nil
}

type TeamResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Badge string `json:"badge"`
	Logo  string `json:"logo,omitempty"`
}

type MatchResponse struct {
	ID       string       `json:"id"`
	LeagueID string       `json:"leagueId"` // competition slug
	League   string       `json:"league"`
	Status   string       `json:"status"`
	Time     string       `json:"time"`
	Date     string       `json:"date"`
	Home     TeamResponse `json:"home"`
	Away     TeamResponse `json:"away"`
	Score    *[2]int      `json:"score"`
}

type EventResponse struct {
	Minute string `json:"minute"`
	Team   string `json:"team"`
	Type   string `json:"type"`
	Player string `json:"player"`
	Assist string `json:"assist,omitempty"` // goals only
	Note   string `json:"note,omitempty"`   // substitutions: who went off
}

type LineupPlayer struct {
	PlayerID string `json:"playerId"`
	Name     string `json:"name"`
	Number   int    `json:"number,omitempty"`
	Starter  bool   `json:"starter"`
}

type Lineups struct {
	Home []LineupPlayer `json:"home"`
	Away []LineupPlayer `json:"away"`
}

type MatchDetailResponse struct {
	MatchResponse
	Events  []EventResponse `json:"events"`
	Stats   json.RawMessage `json:"stats"`
	Lineups Lineups         `json:"lineups"`
}

type ListMatchesResponse struct {
	Data []MatchResponse `json:"data"`
}

type DataResponse struct {
	Data any `json:"data"`
}

// ToMatchResponse renders a match with both teams (the match_rows view).
func ToMatchResponse(m db.MatchRow) MatchResponse {
	kickoff := m.MatchTime.Time.In(domain.WIB)

	resp := MatchResponse{
		ID:       m.ID.String(),
		LeagueID: m.CompetitionSlug,
		League:   m.CompetitionName,
		Status:   m.Status,
		Time:     kickoff.Format("15:04"),
		Date:     kickoff.Format(domain.DateLayout),
		Home:     TeamResponse{ID: m.HomeTeamID.String(), Name: m.HomeName, Badge: m.HomeShortName, Logo: m.HomeLogoUrl.String},
		Away:     TeamResponse{ID: m.AwayTeamID.String(), Name: m.AwayName, Badge: m.AwayShortName, Logo: m.AwayLogoUrl.String},
	}
	if m.Status == "live" {
		resp.Time = liveMinute(m.Stage, m.StageStartedAt, time.Now())
	}
	if m.HomeScore.Valid && m.AwayScore.Valid && m.Status != "scheduled" {
		resp.Score = &[2]int{int(m.HomeScore.Int16), int(m.AwayScore.Int16)}
	}

	return resp
}

// Flashscore stage codes seen in the feeds.
const (
	stageFirstHalf  = 12
	stageSecondHalf = 13
	stageHalfTime   = 38
)

// liveMinute derives the running minute the way Flashscore's own page does:
// from when the current half started, capped at "45+" / "90+" for stoppage time.
func liveMinute(stage pgtype.Int2, startedAt pgtype.Timestamptz, now time.Time) string {
	if stage.Int16 == stageHalfTime {
		return "HT"
	}
	if !stage.Valid || !startedAt.Valid {
		return "LIVE"
	}

	elapsed := int(now.Sub(startedAt.Time).Minutes()) + 1
	switch stage.Int16 {
	case stageFirstHalf:
		if elapsed > 45 {
			return "45+'"
		}
		return strconv.Itoa(max(elapsed, 1)) + "'"
	case stageSecondHalf:
		if elapsed > 45 {
			return "90+'"
		}
		return strconv.Itoa(45+max(elapsed, 1)) + "'"
	}
	return "LIVE"
}

func toMatchDetailResponse(d matchDetail) MatchDetailResponse {
	resp := MatchDetailResponse{
		MatchResponse: ToMatchResponse(d.Match),
		Events:        make([]EventResponse, 0, len(d.Events)),
		Stats:         json.RawMessage(`[]`),
		Lineups:       Lineups{Home: []LineupPlayer{}, Away: []LineupPlayer{}},
	}
	for _, l := range d.Lineups {
		p := LineupPlayer{PlayerID: l.ID.String(), Name: l.Name, Number: int(l.ShirtNumber.Int16), Starter: l.Starter}
		if l.TeamID == d.Match.HomeTeamID {
			resp.Lineups.Home = append(resp.Lineups.Home, p)
		} else {
			resp.Lineups.Away = append(resp.Lineups.Away, p)
		}
	}
	if len(d.Stats) > 0 {
		resp.Stats = d.Stats
	}

	for _, e := range d.Events {
		kind := eventType(e.Type)
		if kind == "" {
			continue
		}
		ev := EventResponse{
			Minute: e.Minute,
			Team:   side(e.TeamID, d.Match.HomeTeamID),
			Type:   kind,
			Player: e.PlayerName,
		}
		switch kind {
		case "goal":
			ev.Assist = e.RelatedPlayerName.String
		case "sub":
			if e.RelatedPlayerName.Valid {
				ev.Note = "Keluar: " + e.RelatedPlayerName.String
			}
		}
		resp.Events = append(resp.Events, ev)
	}

	return resp
}

func side(team, home pgtype.UUID) string {
	if !team.Valid {
		return ""
	}
	if team == home {
		return "home"
	}
	return "away"
}

// eventType maps Flashscore's free-text kinds (IK) to the contract's enum.
func eventType(raw string) string {
	s := strings.ToLower(raw)
	switch {
	case strings.Contains(s, "red"):
		return "red"
	case strings.Contains(s, "yellow"):
		return "yellow"
	case strings.Contains(s, "sub"):
		return "sub"
	case strings.Contains(s, "goal"), s == "penalty":
		return "goal"
	case s == "assistance":
		return "" // the assist belongs to a goal row, not its own event
	}
	return s
}
