package dedup

import "testing"

const (
	// najdorfW and najdorfClocks are one position with different clock
	// fields; najdorfB is the same placement with Black to move.
	najdorfW      = "rnbqkb1r/1p3ppp/p2p1n2/4p3/3NP3/2N5/PPP2PPP/R1BQKB1R w KQkq - 0 7"
	najdorfClocks = "rnbqkb1r/1p3ppp/p2p1n2/4p3/3NP3/2N5/PPP2PPP/R1BQKB1R w KQkq - 5 40"
	najdorfB      = "rnbqkb1r/1p3ppp/p2p1n2/4p3/3NP3/2N5/PPP2PPP/R1BQKB1R b KQkq - 0 7"
)

func TestSetupSetSeen(t *testing.T) {
	var s SetupSet
	cases := []struct {
		fen  string
		want bool
	}{
		{"", false},           // the standard start, first time
		{"", true},            // standard_start_seen
		{najdorfW, false},     // a FEN start, first time
		{najdorfClocks, true}, // the clocks are not part of the position
		{najdorfB, false},     // the side to move is
		{najdorfB, true},      // and now seen
		{"not a fen", false},  // unparsable: never seen...
		{"not a fen", false},  // ...and never registered
		{"", true},            // the standard start is still seen
	}
	for i, c := range cases {
		if got := s.Seen(c.fen); got != c.want {
			t.Errorf("call %d Seen(%q) = %v, want %v", i+1, c.fen, got, c.want)
		}
	}
}
