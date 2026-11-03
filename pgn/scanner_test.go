package pgn

import (
	"strings"
	"testing"
)

const twoGames = `[Event "One"]
[Result "1-0"]

{Start} 1. e4 e5 (1... c5 {Sicilian}) 2. Nf3 $1 {dev} Nc6 1-0

[Event "Two"]

1. d4 *
`

func TestScannerTwoGames(t *testing.T) {
	s := NewScanner(strings.NewReader(twoGames), ParserOptions{})
	var games []*Game
	for s.Next() {
		games = append(games, s.Game())
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d games", len(games))
	}
	g := games[0]
	if ev, _ := g.Tag("Event"); ev != "One" || g.Result != "1-0" {
		t.Fatalf("game 1 header wrong: %+v", g)
	}
	if len(g.PrefixComments) != 1 || g.PrefixComments[0] != "Start" {
		t.Errorf("prefix comments = %v", g.PrefixComments)
	}
	if len(g.Moves) != 4 || g.Moves[0].SAN != "e4" || g.Moves[3].SAN != "Nc6" {
		t.Fatalf("moves = %+v", g.Moves)
	}
	if g.Moves[0].MoveNumber != 1 || g.Moves[1].MoveNumber != 0 || g.Moves[2].MoveNumber != 2 {
		t.Errorf("move numbers wrong")
	}
	v := g.Moves[1].Variations()
	if len(v) != 1 || len(v[0].Moves) != 1 || v[0].Moves[0].SAN != "c5" ||
		len(v[0].Moves[0].Comments()) != 1 {
		t.Fatalf("variation wrong: %+v", v)
	}
	if len(g.Moves[2].NAGs()) != 1 || g.Moves[2].NAGs()[0] != 1 || g.Moves[2].Comments()[0] != "dev" {
		t.Errorf("NAG/comment attachment wrong")
	}
	if games[1].Result != "*" || len(games[1].Moves) != 1 {
		t.Errorf("game 2 wrong: %+v", games[1])
	}
}

func TestScannerMissingResultRecovers(t *testing.T) {
	src := "[Event \"A\"]\n\n1. e4 e5\n\n[Event \"B\"]\n\n1. d4 1-0\n"
	s := NewScanner(strings.NewReader(src), ParserOptions{})
	var games []*Game
	for s.Next() {
		games = append(games, s.Game())
	}
	if len(games) != 2 {
		t.Fatalf("got %d games, want recovery to 2", len(games))
	}
	if len(games[0].ParseErrors()) == 0 {
		t.Error("game 1 should record missing-result parse error")
	}
	if games[0].Result != "" || games[1].Result != "1-0" {
		t.Error("results wrong after recovery")
	}
}

func TestScannerEmptyInput(t *testing.T) {
	s := NewScanner(strings.NewReader(""), ParserOptions{})
	if s.Next() {
		t.Fatal("Next() on empty input must be false")
	}
	if s.Err() != nil {
		t.Fatal("empty input is not an error")
	}
}

// TestScannerLeadingFileCommentIsNotAGame guards against a real divergence
// found differential-testing against pgn-extract's own eco.pgn: a comment
// with no preceding tag section, no moves, and no result (a file-level
// header comment before the first game's tags) must not be surfaced as a
// phantom empty game. The C oracle (grammar.c's parse_game) discards a
// prefix comment seen before the tag list and emits nothing when a "game"
// turns out to have no tags, no moves, and no result; pgn.parser mirrors
// that. Contrast with TestScannerTwoGames's "{Start}" case, a legitimate
// prefix comment that appears after a tag section and must still attach.
func TestScannerLeadingFileCommentIsNotAGame(t *testing.T) {
	src := "{ file header comment, not part of any game }\n\n" +
		"[Event \"Real\"]\n[Result \"*\"]\n\n1. e4 *\n"
	s := NewScanner(strings.NewReader(src), ParserOptions{})
	var games []*Game
	for s.Next() {
		games = append(games, s.Game())
	}
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	if len(games) != 1 {
		t.Fatalf("got %d games, want 1 (leading file comment must not become a phantom game): %+v", len(games), games)
	}
	if ev, _ := games[0].Tag("Event"); ev != "Real" {
		t.Fatalf("wrong game survived: %+v", games[0])
	}
	if len(games[0].PrefixComments) != 0 {
		t.Errorf("leading file comment should not be attached as a prefix comment: %v", games[0].PrefixComments)
	}
}

// A tags-only game (a tag section with no movetext: no moves, no result
// token, nothing discarded) is a record pgn-extract never emits; the CLI
// asks the parser to skip it with SkipTagsOnly so game counts and
// numbering match the oracle (ADR 0002, CONFORMANCE.md). The library
// default keeps the record. Mid-file the question rarely arises: both
// parsers read tags across blank lines, so a stub's tags merge into the
// next game's. A game whose movetext held only stray tokens or a comment
// is not tags-only mid-file (the oracle emits it as a zero-move game),
// but at end of input the oracle stops before dealing with any game that
// has no moves and no result, junk or not.
func TestScannerSkipTagsOnly(t *testing.T) {
	const stub = "[Event \"stub\"]\n[White \"Ghost\"]\n[Result \"1-0\"]\n\n"
	const real = "[Event \"real\"]\n\n1. d4 *\n\n"
	const strayOnly = "[Event \"stray\"]\n\n-\n\n"
	cases := []struct {
		name string
		src  string
		skip bool
		want []string // Event tags of the games yielded
	}{
		{"default keeps a trailing stub", real + stub, false, []string{"real", "stub"}},
		{"default keeps a trailing stub with junk", real + strayOnly, false, []string{"real", "stray"}},
		{"trailing stub skipped", real + stub, true, []string{"real"}},
		{"stub alone yields nothing", stub, true, nil},
		{"two stubs in a row merge and are skipped", stub + stub, true, nil},
		{"trailing stray-only game skipped", real + strayOnly, true, []string{"real"}},
		{"trailing stub with a comment skipped", real + stub + "{c}\n", true, []string{"real"}},
		{"mid-file stray-only game kept", strayOnly + real, true, []string{"stray", "real"}},
		{"mid-file stub with a comment kept", stub + "{c}\n\n" + real, true, []string{"stub", "real"}},
		{"trailing moves without a result kept", real + "[Event \"open\"]\n\n1. e4 e5\n", true, []string{"real", "open"}},
		{"zero-move game with a result kept", stub + "*\n\n" + real, true, []string{"stub", "real"}},
	}
	for _, c := range cases {
		s := NewScanner(strings.NewReader(c.src), ParserOptions{SkipTagsOnly: c.skip})
		var got []string
		for s.Next() {
			ev, _ := s.Game().Tag("Event")
			got = append(got, ev)
		}
		if err := s.Err(); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if strings.Join(got, ",") != strings.Join(c.want, ",") {
			t.Errorf("%s: games = %v, want %v", c.name, got, c.want)
		}
	}

	// A mid-file stub is never seen as a game of its own: its tags run on
	// into the next game's tag section (last occurrence wins), with or
	// without SkipTagsOnly, exactly as in grammar.c.
	for _, skip := range []bool{false, true} {
		s := NewScanner(strings.NewReader(stub+real), ParserOptions{SkipTagsOnly: skip})
		if !s.Next() {
			t.Fatalf("skip=%v: no game", skip)
		}
		g := s.Game()
		ev, _ := g.Tag("Event")
		white, _ := g.Tag("White")
		if ev != "real" || white != "Ghost" || len(g.Moves) != 1 {
			t.Errorf("skip=%v: merged game = Event %q White %q moves %d", skip, ev, white, len(g.Moves))
		}
		if s.Next() {
			t.Errorf("skip=%v: a second game", skip)
		}
	}
}
