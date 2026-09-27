package player

import "testing"

func TestOutcome(t *testing.T) {
	for _, tt := range []struct {
		home, away int
		isHome     bool
		want       string
	}{
		{2, 1, true, "W"}, {2, 1, false, "L"}, {0, 3, false, "W"}, {1, 1, false, "D"},
	} {
		if got := outcome(tt.home, tt.away, tt.isHome); got != tt.want {
			t.Errorf("outcome(%d, %d, home=%v) = %q, want %q", tt.home, tt.away, tt.isHome, got, tt.want)
		}
	}
}
