package sofascore

import (
	"encoding/json"
	"testing"
)

// Incidents from Erlangga FA U17 0-2 RMD U17 (event 16364695), trimmed.
const incidents = `[
 {"text":"FT","time":90,"addedTime":999,"incidentType":"period"},
 {"id":1,"time":59,"isHome":false,"incidentType":"substitution","incidentClass":"regular",
  "playerIn":{"id":2433370,"name":"Ahmad Hafizh Rafi"},"playerOut":{"id":2433374,"name":"Muhammad Rana Syafi'i Pasi"}},
 {"id":2,"time":50,"isHome":false,"incidentType":"card","incidentClass":"yellowRed","player":{"id":2433373,"name":"Muhammad Zainuri"}},
 {"id":3,"time":45,"addedTime":2,"isHome":true,"incidentType":"goal","incidentClass":"penalty","playerName":"Typed Name"},
 {"id":4,"time":38,"isHome":false,"incidentType":"goal","incidentClass":"regular",
  "player":{"id":2433368,"name":"Muhammad Ridho"},"assist1":{"id":7,"name":"Assist Man"}}
]`

func TestIncidentEvent(t *testing.T) {
	var in []Incident
	if err := json.Unmarshal([]byte(incidents), &in); err != nil {
		t.Fatal(err)
	}
	var got []MatchEvent
	for _, i := range in {
		if e, ok := i.Event(); ok {
			got = append(got, e)
		}
	}
	want := []MatchEvent{
		{ID: "1", Type: "Substitution", Minute: "59'", Player: Ref{2433370, "Ahmad Hafizh Rafi"}, Related: Ref{2433374, "Muhammad Rana Syafi'i Pasi"}},
		{ID: "2", Type: "Yellow/Red Card", Minute: "50'", Player: Ref{2433373, "Muhammad Zainuri"}},
		{ID: "3", Type: "Penalty", Minute: "45+2'", Home: true, Player: Ref{Name: "Typed Name"}},
		{ID: "4", Type: "Goal", Minute: "38'", Player: Ref{2433368, "Muhammad Ridho"}, Related: Ref{7, "Assist Man"}},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d events, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("event %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestSeasonYear(t *testing.T) {
	for in, want := range map[string]int16{"2026": 2026, "25/26": 2025} {
		if got := seasonYear(in); got != want {
			t.Errorf("seasonYear(%q) = %d, want %d", in, got, want)
		}
	}
}
