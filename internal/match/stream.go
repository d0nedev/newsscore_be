package match

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/d0nedev/newsscore/internal/platform/stream"

	"github.com/jackc/pgx/v5/pgtype"
)

// NotifyChannel is the Postgres channel the matches triggers publish on (migration 0006).
const NotifyChannel = "match_updates"

type notification struct {
	Kind           string     `json:"kind"`
	ID             string     `json:"id"`
	Status         string     `json:"status"`
	Stage          *int16     `json:"stage"`
	StageStartedAt *time.Time `json:"stage_started_at"`
	HomeScore      *int16     `json:"home_score"`
	AwayScore      *int16     `json:"away_score"`
	Type           string     `json:"type"`
	Minute         string     `json:"minute"`
	Player         string     `json:"player"`
	Team           string     `json:"team"`
}

// ScoreUpdate is the SSE "score" event: enough to patch a match row in place.
type ScoreUpdate struct {
	ID     string  `json:"id"`
	Status string  `json:"status"`
	Time   string  `json:"time"`
	Score  *[2]int `json:"score"`
}

// EventUpdate is the SSE "match_event" event.
type EventUpdate struct {
	MatchID string `json:"matchId"`
	EventResponse
}

// Relay turns trigger payloads into SSE events on hub.
func Relay(hub *stream.Hub, logger *slog.Logger) func(payload string) {
	return func(payload string) {
		var n notification
		if err := json.Unmarshal([]byte(payload), &n); err != nil {
			logger.Error("stream: bad notification", slog.String("payload", payload), slog.Any("error", err))
			return
		}

		event, data := render(n, time.Now())
		if event == "" {
			return
		}
		body, err := json.Marshal(data)
		if err != nil {
			logger.Error("stream: encode event", slog.Any("error", err))
			return
		}
		hub.Publish(event, body)
	}
}

func render(n notification, now time.Time) (string, any) {
	switch n.Kind {
	case "score":
		u := ScoreUpdate{ID: n.ID, Status: n.Status}
		if n.Status == "live" {
			var stage pgtype.Int2
			var started pgtype.Timestamptz
			if n.Stage != nil {
				stage = pgtype.Int2{Int16: *n.Stage, Valid: true}
			}
			if n.StageStartedAt != nil {
				started = pgtype.Timestamptz{Time: *n.StageStartedAt, Valid: true}
			}
			u.Time = liveMinute(stage, started, now)
		}
		if n.Status != "scheduled" && n.HomeScore != nil && n.AwayScore != nil {
			u.Score = &[2]int{int(*n.HomeScore), int(*n.AwayScore)}
		}
		return "score", u

	case "match_event":
		kind := eventType(n.Type)
		if kind == "" {
			return "", nil
		}
		return "match_event", EventUpdate{
			MatchID:       n.ID,
			EventResponse: EventResponse{Minute: n.Minute, Team: n.Team, Type: kind, Player: n.Player},
		}
	}
	return "", nil
}
