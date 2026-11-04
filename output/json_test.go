package output

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// TestWriteJSONGame is pinned byte-for-byte against the C oracle:
//
//	pgn-extract --json --nomovenumbers game.pgn
//
// (--nomovenumbers because the C tool's move numbers leak into its JSON
// output through its line buffer once a game is long enough; see
// CONFORMANCE.md.) Comments, the variation and the result token are
// left out, the NAGs become a "nags" array, and the tag values are
// written exactly as the PGN output writes them.
func TestWriteJSONGame(t *testing.T) {
	src := `[Event "Test \"Open\""]
[Site "Here"]
[Date "2020.01.01"]
[Round "1"]
[White "A, B"]
[Black "C"]
[Result "1-0"]
[WhiteElo "2000"]
[Custom "x\\y"]

{ prefix } 1. e4 $1 { good } e5 $3 (1... c5 2. Nf3) 2. Nf3 {a} Nc6 3. Bb5 a6 1-0
`
	g, r := gameAndReplay(t, src)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN, JSON: true})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	want := `[
{
"Event" : "Test \"Open\"",
"Site" : "Here",
"Date" : "2020.01.01",
"Round" : "1",
"White" : "A, B",
"Black" : "C",
"Result" : "1-0",
"WhiteElo" : "2000",
"Custom" : "x\\y",

"Moves":[{ "move" : "e4", "nags" : ["$1"]  }, { "move" : "e5", "nags" : ["$3"]  }, { "move" : "Nf3" }, { "move" : "Nc6" }, { "move" : "Bb5" }, { "move" : "a6" }]

}
]
`
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

// jsonSample is three games: a mated game with a NAG, a game from a FEN
// starting with black to move at move 40, and a zero-move game.
const jsonSample = `[Event "One"]
[Result "1-0"]

1. e4 $1 e5 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7# 1-0

[Event "Two"]
[Result "*"]
[SetUp "1"]
[FEN "4k3/8/8/8/8/8/4P3/4K3 b - - 0 40"]

40... Kd7 41. e4 Kd6 42. e5+ *

[Event "Zero"]
[Result "1/2-1/2"]

1/2-1/2
`

// writeJSON writes every game of src through one Writer with opts (JSON
// forced on) and closes it, returning the output.
func writeJSON(t *testing.T, src string, opts Options) string {
	t.Helper()
	opts.JSON = true
	var buf bytes.Buffer
	w := NewWriter(&buf, opts)
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	for s.Next() {
		g := s.Game()
		r, err := replay.Game(g, replay.Options{AllowNullMoves: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := w.WriteGame(g, r); err != nil {
			t.Fatal(err)
		}
	}
	if s.Err() != nil {
		t.Fatal(s.Err())
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// movesLines returns the "Moves" line of every game in out.
func movesLines(out string) []string {
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, `"Moves"`) {
			lines = append(lines, l)
		}
	}
	return lines
}

// TestWriteJSONArray pins the framing of several games against the C
// oracle (pgn-extract --json --nomovenumbers --notags): "[" before the
// first game, "," between games, "]" after the last, and an empty array
// for a zero-move game.
func TestWriteJSONArray(t *testing.T) {
	got := writeJSON(t, jsonSample, Options{Format: SAN, NoTags: true})
	want := `[
{
"Moves":[{ "move" : "e4", "nags" : ["$1"]  }, { "move" : "e5" }, { "move" : "Qh5" }, { "move" : "Nc6" }, { "move" : "Bc4" }, { "move" : "Nf6" }, { "move" : "Qxf7#" }]

},
{
"Moves":[{ "move" : "Kd7" }, { "move" : "e4" }, { "move" : "Kd6" }, { "move" : "e5+" }]

},
{
"Moves":[]

}
]
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestWriteJSONNoGames: the C tool writes nothing, not "[]", when no game
// reaches the output.
func TestWriteJSONNoGames(t *testing.T) {
	if got := writeJSON(t, "", Options{Format: SAN}); got != "" {
		t.Errorf("got %q, want nothing", got)
	}
}

// TestWriteJSONMoveFields pins the per-move fields against the C oracle,
// each case being pgn-extract --json --nomovenumbers with the flags in
// its name on jsonSample; only the "Moves" lines are compared.
func TestWriteJSONMoveFields(t *testing.T) {
	cases := []struct {
		name string
		opts Options
		want []string
	}{
		{"-F", Options{Format: SAN, FinalFEN: true}, []string{
			`"Moves":[{ "move" : "e4", "nags" : ["$1"]  }, { "move" : "e5" }, { "move" : "Qh5" }, { "move" : "Nc6" }, { "move" : "Bc4" }, { "move" : "Nf6" }, { "move" : "Qxf7#", "FEN" : "r1bqkb1r/pppp1Qpp/2n2n2/4p3/2B1P3/8/PPPP1PPP/RNB1K1NR b KQkq - 0 4"  }]`,
			`"Moves":[{ "move" : "Kd7" }, { "move" : "e4" }, { "move" : "Kd6" }, { "move" : "e5+", "FEN" : "8/8/3k4/4P3/8/8/8/4K3 b - - 0 42"  }]`,
			`"Moves":[]`,
		}},
		{"--fencomments", Options{Format: SAN, FENComments: true}, []string{
			`"Moves":[{ "move" : "e4", "nags" : ["$1"] , "FEN" : "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1" }, { "move" : "e5", "FEN" : "rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2" }, { "move" : "Qh5", "FEN" : "rnbqkbnr/pppp1ppp/8/4p2Q/4P3/8/PPPP1PPP/RNB1KBNR b KQkq - 1 2" }, { "move" : "Nc6", "FEN" : "r1bqkbnr/pppp1ppp/2n5/4p2Q/4P3/8/PPPP1PPP/RNB1KBNR w KQkq - 2 3" }, { "move" : "Bc4", "FEN" : "r1bqkbnr/pppp1ppp/2n5/4p2Q/2B1P3/8/PPPP1PPP/RNB1K1NR b KQkq - 3 3" }, { "move" : "Nf6", "FEN" : "r1bqkb1r/pppp1ppp/2n2n2/4p2Q/2B1P3/8/PPPP1PPP/RNB1K1NR w KQkq - 4 4" }, { "move" : "Qxf7#", "FEN" : "r1bqkb1r/pppp1Qpp/2n2n2/4p3/2B1P3/8/PPPP1PPP/RNB1K1NR b KQkq - 0 4" }]`,
			`"Moves":[{ "move" : "Kd7", "FEN" : "8/3k4/8/8/8/8/4P3/4K3 w - - 1 41" }, { "move" : "e4", "FEN" : "8/3k4/8/8/4P3/8/8/4K3 b - e3 0 41" }, { "move" : "Kd6", "FEN" : "8/8/3k4/8/4P3/8/8/4K3 w - - 1 42" }, { "move" : "e5+", "FEN" : "8/8/3k4/4P3/8/8/8/4K3 b - - 0 42" }]`,
			`"Moves":[]`,
		}},
		// -F adds nothing when --fencomments already wrote the final FEN.
		{"--fencomments -F", Options{Format: SAN, FENComments: true, FinalFEN: true}, []string{
			`"Moves":[{ "move" : "e4", "nags" : ["$1"] , "FEN" : "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1" }, { "move" : "e5", "FEN" : "rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2" }, { "move" : "Qh5", "FEN" : "rnbqkbnr/pppp1ppp/8/4p2Q/4P3/8/PPPP1PPP/RNB1KBNR b KQkq - 1 2" }, { "move" : "Nc6", "FEN" : "r1bqkbnr/pppp1ppp/2n5/4p2Q/4P3/8/PPPP1PPP/RNB1KBNR w KQkq - 2 3" }, { "move" : "Bc4", "FEN" : "r1bqkbnr/pppp1ppp/2n5/4p2Q/2B1P3/8/PPPP1PPP/RNB1K1NR b KQkq - 3 3" }, { "move" : "Nf6", "FEN" : "r1bqkb1r/pppp1ppp/2n2n2/4p2Q/2B1P3/8/PPPP1PPP/RNB1K1NR w KQkq - 4 4" }, { "move" : "Qxf7#", "FEN" : "r1bqkb1r/pppp1Qpp/2n2n2/4p3/2B1P3/8/PPPP1PPP/RNB1K1NR b KQkq - 0 4" }]`,
			`"Moves":[{ "move" : "Kd7", "FEN" : "8/3k4/8/8/8/8/4P3/4K3 w - - 1 41" }, { "move" : "e4", "FEN" : "8/3k4/8/8/4P3/8/8/4K3 b - e3 0 41" }, { "move" : "Kd6", "FEN" : "8/8/3k4/8/4P3/8/8/4K3 w - - 1 42" }, { "move" : "e5+", "FEN" : "8/8/3k4/4P3/8/8/8/4K3 b - - 0 42" }]`,
			`"Moves":[]`,
		}},
		{"--hashcomments", Options{Format: SAN, HashComments: true}, []string{
			`"Moves":[{ "move" : "e4", "nags" : ["$1"] , "HashCode" : "823c9b50fd114196" }, { "move" : "e5", "HashCode" : "0844931a6ef4b9a0" }, { "move" : "Qh5", "HashCode" : "a849703c42999f3c" }, { "move" : "Nc6", "HashCode" : "03a4a8de53083a3f" }, { "move" : "Bc4", "HashCode" : "60171cf67b4bec99" }, { "move" : "Nf6", "HashCode" : "3bf92803d6b1ad0f" }, { "move" : "Qxf7#", "HashCode" : "c3116e611017a62f" }]`,
			`"Moves":[{ "move" : "Kd7", "HashCode" : "f53783a0accd67ca" }, { "move" : "e4", "HashCode" : "31308ee8474ddac0" }, { "move" : "Kd6", "HashCode" : "8c5d26847799627a" }, { "move" : "e5+", "HashCode" : "ed2430797387441e" }]`,
			`"Moves":[]`,
		}},
		{"--evaluation", Options{Format: SAN, Evaluation: true}, []string{
			`"Moves":[{ "move" : "e4", "nags" : ["$1"] , "evaluation" : "1.00" }, { "move" : "e5", "evaluation" : "0.00" }, { "move" : "Qh5", "evaluation" : "1.30" }, { "move" : "Nc6", "evaluation" : "1.10" }, { "move" : "Bc4", "evaluation" : "1.50" }, { "move" : "Nf6", "evaluation" : "1.50" }, { "move" : "Qxf7#", "evaluation" : "5.00" }]`,
			`"Moves":[{ "move" : "Kd7", "evaluation" : "0.80" }, { "move" : "e4", "evaluation" : "0.80" }, { "move" : "Kd6", "evaluation" : "0.90" }, { "move" : "e5+", "evaluation" : "0.90" }]`,
			`"Moves":[]`,
		}},
		{"--evaluation --hashcomments -F", Options{Format: SAN, Evaluation: true, HashComments: true, FinalFEN: true}, []string{
			`"Moves":[{ "move" : "e4", "nags" : ["$1"] , "evaluation" : "1.00", "HashCode" : "823c9b50fd114196" }, { "move" : "e5", "evaluation" : "0.00", "HashCode" : "0844931a6ef4b9a0" }, { "move" : "Qh5", "evaluation" : "1.30", "HashCode" : "a849703c42999f3c" }, { "move" : "Nc6", "evaluation" : "1.10", "HashCode" : "03a4a8de53083a3f" }, { "move" : "Bc4", "evaluation" : "1.50", "HashCode" : "60171cf67b4bec99" }, { "move" : "Nf6", "evaluation" : "1.50", "HashCode" : "3bf92803d6b1ad0f" }, { "move" : "Qxf7#", "evaluation" : "5.00", "HashCode" : "c3116e611017a62f", "FEN" : "r1bqkb1r/pppp1Qpp/2n2n2/4p3/2B1P3/8/PPPP1PPP/RNB1K1NR b KQkq - 0 4"  }]`,
			`"Moves":[{ "move" : "Kd7", "evaluation" : "0.80", "HashCode" : "f53783a0accd67ca" }, { "move" : "e4", "evaluation" : "0.80", "HashCode" : "31308ee8474ddac0" }, { "move" : "Kd6", "evaluation" : "0.90", "HashCode" : "8c5d26847799627a" }, { "move" : "e5+", "evaluation" : "0.90", "HashCode" : "ed2430797387441e", "FEN" : "8/8/3k4/4P3/8/8/8/4K3 b - - 0 42"  }]`,
			`"Moves":[]`,
		}},
		// The ply limit counts from the FEN's move number: the second
		// game's first move is ply 80, and a cut game gets no final FEN.
		{"--plylimit 82 -F", Options{Format: SAN, PlyLimit: 82, HasPlyLimit: true, FinalFEN: true}, []string{
			`"Moves":[{ "move" : "e4", "nags" : ["$1"]  }, { "move" : "e5" }, { "move" : "Qh5" }, { "move" : "Nc6" }, { "move" : "Bc4" }, { "move" : "Nf6" }, { "move" : "Qxf7#", "FEN" : "r1bqkb1r/pppp1Qpp/2n2n2/4p3/2B1P3/8/PPPP1PPP/RNB1K1NR b KQkq - 0 4"  }]`,
			`"Moves":[{ "move" : "Kd7" }, { "move" : "e4" }, { "move" : "Kd6" }]`,
			`"Moves":[]`,
		}},
		// -Wuci drops the NAGs too, as it does in PGN output.
		{"-Wuci", Options{Format: UCI}, []string{
			`"Moves":[{ "move" : "e2e4" }, { "move" : "e7e5" }, { "move" : "d1h5" }, { "move" : "b8c6" }, { "move" : "f1c4" }, { "move" : "g8f6" }, { "move" : "h5f7" }]`,
			`"Moves":[{ "move" : "e8d7" }, { "move" : "e2e4" }, { "move" : "d7d6" }, { "move" : "e4e5" }]`,
			`"Moves":[]`,
		}},
		{"-Wxlalg", Options{Format: XLAlg}, []string{
			`"Moves":[{ "move" : "e2-e4", "nags" : ["$1"]  }, { "move" : "e7-e5" }, { "move" : "Qd1-h5" }, { "move" : "Nb8-c6" }, { "move" : "Bf1-c4" }, { "move" : "Ng8-f6" }, { "move" : "Qh5xf7#" }]`,
			`"Moves":[{ "move" : "Ke8-d7" }, { "move" : "e2-e4" }, { "move" : "Kd7-d6" }, { "move" : "e4-e5+" }]`,
			`"Moves":[]`,
		}},
		{"--nochecks", Options{Format: SAN, NoChecks: true}, []string{
			`"Moves":[{ "move" : "e4", "nags" : ["$1"]  }, { "move" : "e5" }, { "move" : "Qh5" }, { "move" : "Nc6" }, { "move" : "Bc4" }, { "move" : "Nf6" }, { "move" : "Qxf7" }]`,
			`"Moves":[{ "move" : "Kd7" }, { "move" : "e4" }, { "move" : "Kd6" }, { "move" : "e5" }]`,
			`"Moves":[]`,
		}},
		{"-N", Options{Format: SAN, NoNAGs: true}, []string{
			`"Moves":[{ "move" : "e4" }, { "move" : "e5" }, { "move" : "Qh5" }, { "move" : "Nc6" }, { "move" : "Bc4" }, { "move" : "Nf6" }, { "move" : "Qxf7#" }]`,
			`"Moves":[{ "move" : "Kd7" }, { "move" : "e4" }, { "move" : "Kd6" }, { "move" : "e5+" }]`,
			`"Moves":[]`,
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := movesLines(writeJSON(t, jsonSample, c.opts))
			if len(got) != len(c.want) {
				t.Fatalf("got %d Moves lines, want %d:\n%s", len(got), len(c.want), strings.Join(got, "\n"))
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("game %d:\ngot:  %s\nwant: %s", i+1, got[i], c.want[i])
				}
			}
		})
	}
}

// TestWriteJSONRoster: --seven-tag-roster keeps the FEN tags of a
// FEN game (output.c print_algebraic_game), and the tag section is
// followed by a blank line, as in pgn-extract --json --nomovenumbers -7.
func TestWriteJSONRoster(t *testing.T) {
	got := writeJSON(t, jsonSample, Options{Format: SAN, SevenTagRoster: true})
	want := `[
{
"Event" : "One",
"Site" : "?",
"Date" : "????.??.??",
"Round" : "?",
"White" : "?",
"Black" : "?",
"Result" : "1-0",

"Moves":[{ "move" : "e4", "nags" : ["$1"]  }, { "move" : "e5" }, { "move" : "Qh5" }, { "move" : "Nc6" }, { "move" : "Bc4" }, { "move" : "Nf6" }, { "move" : "Qxf7#" }]

},
{
"Event" : "Two",
"Site" : "?",
"Date" : "????.??.??",
"Round" : "?",
"White" : "?",
"Black" : "?",
"Result" : "*",
"SetUp" : "1",
"FEN" : "4k3/8/8/8/8/8/4P3/4K3 b - - 0 40",

"Moves":[{ "move" : "Kd7" }, { "move" : "e4" }, { "move" : "Kd6" }, { "move" : "e5+" }]

},
{
"Event" : "Zero",
"Site" : "?",
"Date" : "????.??.??",
"Round" : "?",
"White" : "?",
"Black" : "?",
"Result" : "1/2-1/2",

"Moves":[]

}
]
`
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestWriteJSONConsecutiveNAGs is a documented divergence (CONFORMANCE.md):
// the C tool writes NAGs that follow one another directly with no comma
// between them (["$1""$2"]), which is not JSON; pgnx separates them. The
// null move is "--" in both.
func TestWriteJSONConsecutiveNAGs(t *testing.T) {
	got := movesLines(writeJSON(t, "[Result \"*\"]\n\n1. e4 $1 $2 -- 2. d4 *\n", Options{Format: SAN}))
	want := `"Moves":[{ "move" : "e4", "nags" : ["$1", "$2"]  }, { "move" : "--" }, { "move" : "d4" }]`
	if len(got) != 1 || got[0] != want {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(got, "\n"), want)
	}
}

// TestWriteJSONBrokenGame: a game written without a replay is rendered
// from its source moves, as Source format is for PGN output.
func TestWriteJSONBrokenGame(t *testing.T) {
	s := pgn.NewScanner(strings.NewReader("[Result \"*\"]\n\n1. e4 e5 2. Qh4 Nc6 *\n"), pgn.ParserOptions{})
	if !s.Next() {
		t.Fatal(s.Err())
	}
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: LAlg, JSON: true, NoTags: true})
	if err := w.WriteGame(s.Game(), nil); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	want := "[\n{\n\"Moves\":[{ \"move\" : \"e4\" }, { \"move\" : \"e5\" }, { \"move\" : \"Qh4\" }, { \"move\" : \"Nc6\" }]\n\n}\n]\n"
	if buf.String() != want {
		t.Errorf("got:\n%s\nwant:\n%s", buf.String(), want)
	}
}

// TestWriteJSONPositionFormats: EPD and FEN have no JSON form.
func TestWriteJSONPositionFormats(t *testing.T) {
	g, r := gameAndReplay(t, fools)
	for _, f := range []Format{EPD, FEN} {
		var buf bytes.Buffer
		w := NewWriter(&buf, Options{Format: f, JSON: true})
		if err := w.WriteGame(g, r); err == nil {
			t.Errorf("%v: no error", f)
		}
		if buf.Len() != 0 {
			t.Errorf("%v: wrote %q", f, buf.String())
		}
	}
}

// TestWriteJSONCloseIsNoOpForPGN: Close writes nothing for PGN output,
// so destinations can always call it.
func TestWriteJSONCloseIsNoOpForPGN(t *testing.T) {
	g, r := gameAndReplay(t, fools)
	var buf bytes.Buffer
	w := NewWriter(&buf, Options{Format: SAN})
	if err := w.WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	n := buf.Len()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != n {
		t.Errorf("Close wrote %q", buf.String()[n:])
	}
}
