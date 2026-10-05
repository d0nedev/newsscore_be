package ingest

import "testing"

func TestGroupName(t *testing.T) {
	for _, c := range []struct{ table, competition, want string }{
		{"Liga Topskor U17, Group A 2026", "Liga Topskor U17", "Group A"},
		{"3rd Place  2026", "Liga Topskor U17", "3rd Place"},
		{"Liga Topskor U17, Second Round, Group 1 2026", "Liga Topskor U17", "Second Round, Group 1"},
		{"Indonesia Nusantara League 25/26", "Indonesia Nusantara League", ""},
		{"Indonesia Nusantara League 2026", "Indonesia Nusantara League", ""},
	} {
		if got := groupName(c.table, c.competition); got != c.want {
			t.Errorf("groupName(%q) = %q, want %q", c.table, got, c.want)
		}
	}
}
