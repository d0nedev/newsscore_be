package match

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"
	"uuid"

	"github.com/d0nedev/newsscore/internal/platform/apperror"
	db "github.com/d0nedev/newsscore/internal/platform/database/sqlc"

	"github.com/jackc/pgx/v5/pgtype"
)

// dateLayout is the contract's "DD.MM.YYYY" (see docs/development-plan/api-contract-plan.md).
const dateLayout = "02.01.2006"

// wib is the product's home time zone: a "date" means a calendar day in Indonesia.
var wib = time.FixedZone("WIB", 7*60*60)

var validStatuses = map[string]bool{"scheduled": true, "live": true, "finished": true}

type listFilter struct {
	From   time.Time
	TeamID *uuid.UUID
	Status string
}

func parseListFilter(q url.Values) (listFilter, error) {
	var f listFilter

	date := q.Get("date")
	if date == "" {
		now := time.Now().In(wib)
		f.From = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, wib)
	} else {
		from, err := time.ParseInLocation(dateLayout, date, wib)
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
	ID     string       `json:"id"`
	Status string       `json:"status"`
	Time   string       `json:"time"`
	Date   string       `json:"date"`
	Home   TeamResponse `json:"home"`
	Away   TeamResponse `json:"away"`
	Score  *[2]int      `json:"score"`
}

type EventResponse struct {
	Minute string `json:"minute"`
	Team   string `json:"team"`
	Type   string `json:"type"`
	Player string `json:"player"`
}

type MatchDetailResponse struct {
	MatchResponse
	Events []EventResponse `json:"events"`
	Stats  json.RawMessage `json:"stats"`
}

type ListMatchesResponse struct {
	Data []MatchResponse `json:"data"`
}

type DataResponse struct {
	Data any `json:"data"`
}

func toMatchResponse(m db.ListMatchesRow) MatchResponse {
	kickoff := m.MatchTime.Time.In(wib)

	resp := MatchResponse{
		ID:     m.ID.String(),
		Status: m.Status,
		Time:   kickoff.Format("15:04"),
		Date:   kickoff.Format(dateLayout),
		Home:   TeamResponse{ID: m.HomeID.String(), Name: m.HomeName, Badge: m.HomeShortName, Logo: m.HomeLogoUrl.String},
		Away:   TeamResponse{ID: m.AwayID.String(), Name: m.AwayName, Badge: m.AwayShortName, Logo: m.AwayLogoUrl.String},
	}
	// ponytail: live minute is not stored yet, so live matches show "LIVE" until the ingestor records it.
	if m.Status == "live" {
		resp.Time = "LIVE"
	}
	if m.HomeScore.Valid && m.AwayScore.Valid && m.Status != "scheduled" {
		resp.Score = &[2]int{int(m.HomeScore.Int16), int(m.AwayScore.Int16)}
	}

	return resp
}

func toMatchDetailResponse(d matchDetail) MatchDetailResponse {
	resp := MatchDetailResponse{
		MatchResponse: toMatchResponse(d.Match),
		Events:        make([]EventResponse, 0, len(d.Events)),
		Stats:         json.RawMessage(`[]`),
	}
	if len(d.Stats) > 0 {
		resp.Stats = d.Stats
	}

	for _, e := range d.Events {
		kind := eventType(e.Type)
		if kind == "" {
			continue
		}
		resp.Events = append(resp.Events, EventResponse{
			Minute: e.Minute,
			Team:   side(e.TeamID, d.Match.HomeID),
			Type:   kind,
			Player: e.PlayerName,
		})
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
