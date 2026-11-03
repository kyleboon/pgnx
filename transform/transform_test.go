package transform

import (
	"errors"
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

func parse(t *testing.T, src string) (*pgn.Game, *replay.Result) {
	t.Helper()
	s := pgn.NewScanner(strings.NewReader(src), pgn.ParserOptions{})
	if !s.Next() {
		t.Fatalf("parse: %v", s.Err())
	}
	g := s.Game()
	r, err := replay.Game(g, replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return g, r
}

func tag(t *testing.T, g *pgn.Game, name string) string {
	t.Helper()
	v, ok := g.Tag(name)
	if !ok {
		t.Fatalf("no %s tag in %v", name, g.Tags)
	}
	return v
}

// Every HashCode value below is the C oracle's:
//
//	printf '...' | pgn-extract --addhashcode --quiet
func TestHashCodeTag(t *testing.T) {
	cases := []struct{ src, want string }{
		{"[Result \"*\"]\n\n1. e4 e5 *\n", "3d752b1e"},
		{"[Result \"1-0\"]\n\n1. d4 d5 1-0\n", "9cd32e5e"},
		// A variation does not contribute.
		{"[Result \"*\"]\n\n{pre} 1. e4 {c1} e5 $1 {c2} (1... c5 {alt} 2. Nf3) 2. Nf3 Nc6 *\n", "c580b5f7"},
		// A FEN start hashes from the FEN board; the start position itself is not summed.
		{"[Result \"*\"]\n[FEN \"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1\"]\n\n1... e5 2. Nf3 *\n", "d994d364"},
	}
	for _, c := range cases {
		g, r := parse(t, c.src)
		if _, err := Apply(g, r, Options{HashCode: true}); err != nil {
			t.Fatal(err)
		}
		if got := tag(t, g, "HashCode"); got != c.want {
			t.Errorf("%q: HashCode = %s, want %s", c.src, got, c.want)
		}
	}
}

// pgn-extract rewrites a HashCode tag the game already carries even when
// --addhashcode is not given (output.c format_game), and leaves other
// tags alone.
func TestHashCodeTagPresentIsRewritten(t *testing.T) {
	g, r := parse(t, "[Result \"*\"]\n[HashCode \"deadbeef\"]\n[PlyCount \"99\"]\n\n1. e4 e5 *\n")
	if _, err := Apply(g, r, Options{}); err != nil {
		t.Fatal(err)
	}
	if got := tag(t, g, "HashCode"); got != "3d752b1e" {
		t.Errorf("HashCode = %s, want 3d752b1e", got)
	}
	if got := tag(t, g, "PlyCount"); got != "99" {
		t.Errorf("PlyCount = %s, want 99 (untouched)", got)
	}
}

// pgn-extract --plycount --totalplycount; TotalPlyCount counts variation
// plies only while variations are kept (-V makes it equal PlyCount).
func TestPlyCountTags(t *testing.T) {
	src := "[Result \"*\"]\n\n{pre} 1. e4 {c1} e5 $1 {c2} (1... c5 {alt} 2. Nf3) 2. Nf3 Nc6 *\n"
	g, r := parse(t, src)
	if _, err := Apply(g, r, Options{PlyCount: true, TotalPlyCount: true, CountVariations: true}); err != nil {
		t.Fatal(err)
	}
	if got := tag(t, g, "PlyCount"); got != "4" {
		t.Errorf("PlyCount = %s, want 4", got)
	}
	if got := tag(t, g, "TotalPlyCount"); got != "6" {
		t.Errorf("TotalPlyCount = %s, want 6", got)
	}

	g, r = parse(t, src)
	if _, err := Apply(g, r, Options{PlyCount: true, TotalPlyCount: true}); err != nil {
		t.Fatal(err)
	}
	if got := tag(t, g, "TotalPlyCount"); got != "4" {
		t.Errorf("TotalPlyCount without variations = %s, want 4", got)
	}
}

func TestDropPlies(t *testing.T) {
	const game = "[Event \"y\"]\n[Result \"1-0\"]\n\n{pre} 1. d4 d5 2. c4 e6 3. Nc3 Nf6 1-0\n"
	cases := []struct {
		name     string
		drop     int
		wantFEN  string
		wantSAN  string // first remaining move
		wantLeft int
		keepPre  bool
	}{
		// pgn-extract --dropply 3: the position after three plies, prefix comment gone.
		{"positive", 3, "rnbqkbnr/ppp1pppp/8/3p4/2PP4/8/PP2PPPP/RNBQKBNR b KQkq c3 0 2", "e6", 3, false},
		// --dropply -2 keeps the last two plies.
		{"negative", -2, "rnbqkbnr/ppp2ppp/4p3/3p4/2PP4/8/PP2PPPP/RNBQKBNR w KQkq - 0 3", "Nc3", 2, false},
		// --dropply -6 on a six-ply game drops nothing but still pins the start as a FEN.
		{"zero", -6, "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1", "d4", 6, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g, r := parse(t, game)
			nr, err := Apply(g, r, Options{DropPlies: c.drop})
			if err != nil {
				t.Fatal(err)
			}
			if got := tag(t, g, "FEN"); got != c.wantFEN {
				t.Errorf("FEN = %s, want %s", got, c.wantFEN)
			}
			if got := tag(t, g, "SetUp"); got != "1" {
				t.Errorf("SetUp = %s, want 1", got)
			}
			if len(g.Moves) != c.wantLeft || g.Moves[0].SAN != c.wantSAN {
				t.Errorf("moves left = %d starting %q, want %d starting %q", len(g.Moves), g.Moves[0].SAN, c.wantLeft, c.wantSAN)
			}
			if (len(g.PrefixComments) > 0) != c.keepPre {
				t.Errorf("prefix comments kept = %v, want %v", len(g.PrefixComments) > 0, c.keepPre)
			}
			if nr == nil || len(nr.Moves) != c.wantLeft || nr.Positions[0].FEN != c.wantFEN {
				t.Errorf("replay not rebuilt from the new start: %+v", nr)
			}
		})
	}
}

func TestDropPliesZeroIsNoop(t *testing.T) {
	g, r := parse(t, "[Result \"*\"]\n\n{pre} 1. e4 e5 *\n")
	if _, err := Apply(g, r, Options{DropPlies: 0}); err != nil {
		t.Fatal(err)
	}
	if _, ok := g.Tag("FEN"); ok {
		t.Error("--dropply 0 must not add a FEN tag")
	}
}

// A game too short to drop from is a ShortGameError, and nothing about
// the game has changed when it is returned.
func TestDropPliesTooShort(t *testing.T) {
	for _, drop := range []int{3, -3} {
		g, r := parse(t, "[Result \"*\"]\n\n1. e4 e5 *\n")
		_, err := Apply(g, r, Options{DropPlies: drop, LineNumbers: "L", PlyCount: true})
		var short *ShortGameError
		if !errors.As(err, &short) || short.Have != 2 || short.Plies != 3 || short.Keep != (drop < 0) {
			t.Errorf("drop %d of 2 plies: err = %v", drop, err)
		}
		if len(g.PrefixComments) != 0 || len(g.Tags) != 1 || len(g.Moves) != 2 {
			t.Errorf("drop %d: game changed before the error: %v %v", drop, g.PrefixComments, g.Tags)
		}
	}
}

// With KeepShort the short game is kept as pgn-extract --keepbroken keeps
// it: untouched by the drop, the other transforms applied, the prefix
// comments gone when plies were to be dropped (oracle: pgn-extract
// --dropply 5 --plycount --keepbroken) but kept when they were to be
// kept; the error still comes back for reporting.
func TestDropPliesKeepShort(t *testing.T) {
	for _, c := range []struct {
		drop    int
		wantPre int
	}{{5, 0}, {-5, 1}} {
		g, r := parse(t, "[Result \"*\"]\n\n{pre} 1. e4 e5 *\n")
		nr, err := Apply(g, r, Options{DropPlies: c.drop, PlyCount: true, KeepShort: true})
		var short *ShortGameError
		if !errors.As(err, &short) || nr != r {
			t.Errorf("drop %d: err %v, replay kept %v", c.drop, err, nr == r)
		}
		if _, ok := g.Tag("FEN"); ok || len(g.Moves) != 2 {
			t.Errorf("drop %d: game was cut", c.drop)
		}
		if got := tag(t, g, "PlyCount"); got != "2" {
			t.Errorf("drop %d: PlyCount = %s, want 2", c.drop, got)
		}
		if len(g.PrefixComments) != c.wantPre {
			t.Errorf("drop %d: prefix comments %v, want %d", c.drop, g.PrefixComments, c.wantPre)
		}
	}
}

// --dropply recomputes PlyCount/TotalPlyCount when the game carries them
// or they were asked for (apply.c drop_plies_from_start).
func TestDropPliesRecountsPlies(t *testing.T) {
	g, r := parse(t, "[Result \"*\"]\n[PlyCount \"4\"]\n\n1. e4 e5 2. Nf3 Nc6 *\n")
	if _, err := Apply(g, r, Options{DropPlies: 1}); err != nil {
		t.Fatal(err)
	}
	if got := tag(t, g, "PlyCount"); got != "3" {
		t.Errorf("PlyCount = %s, want 3", got)
	}
}

// --dropbefore drops every ply up to and including the move whose comment
// equals the pattern (whitespace-trimmed, exact). For a game that starts
// from a FEN the C tool counts from the move numbers, not from the game's
// first move, so here three of the four moves go.
func TestDropBefore(t *testing.T) {
	g, r := parse(t, "[Result \"*\"]\n\n1. e4 e5 { my  cmt } 2. Nf3 Nc6 *\n")
	if _, err := Apply(g, r, Options{DropBefore: "my  cmt"}); err != nil {
		t.Fatal(err)
	}
	if len(g.Moves) != 2 || g.Moves[0].SAN != "Nf3" {
		t.Errorf("want Nf3 Nc6 left, got %d moves", len(g.Moves))
	}

	g, r = parse(t, "[Result \"*\"]\n\n1. e4 e5 { my  cmt } 2. Nf3 Nc6 *\n")
	if _, err := Apply(g, r, Options{DropBefore: "my cmt"}); err != nil {
		t.Fatal(err)
	}
	if len(g.Moves) != 4 {
		t.Errorf("a non-identical pattern must not match; got %d moves", len(g.Moves))
	}

	g, r = parse(t, "[Result \"1/2-1/2\"]\n[SetUp \"1\"]\n[FEN \"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq - 0 1\"]\n\n1... e5 2. Nf3 {cmt} Nc6 3. Bb5 1/2-1/2\n")
	if _, err := Apply(g, r, Options{DropBefore: "cmt", PlyCount: true}); err != nil {
		t.Fatal(err)
	}
	if got := tag(t, g, "FEN"); got != "r1bqkbnr/pppp1ppp/2n5/4p3/4P3/5N2/PPPP1PPP/RNBQKB1R w KQkq - 2 3" {
		t.Errorf("FEN = %s", got)
	}
	if got := tag(t, g, "PlyCount"); got != "1" {
		t.Errorf("PlyCount = %s, want 1", got)
	}
}

// pgn-extract --linenumbers L prepends "{ L:start:end }" to the prefix comments.
func TestLineNumbers(t *testing.T) {
	g, r := parse(t, "[Event \"x\"]\n[Result \"*\"]\n\n{pre} 1. e4 e5 *\n")
	if _, err := Apply(g, r, Options{LineNumbers: "L"}); err != nil {
		t.Fatal(err)
	}
	if len(g.PrefixComments) != 2 || g.PrefixComments[0] != "L:1:4" {
		t.Errorf("prefix comments = %q, want [L:1:4 pre]", g.PrefixComments)
	}
}

// --addelotags / --addfideidtags look the White and Black names up in a
// tab-separated file; a missing player adds nothing.
func TestPlayerTags(t *testing.T) {
	ratings, err := LoadPlayerInfo(strings.NewReader("Barnes, David J.\t2000\nHorton, Mark\t1900\nignored line\n"))
	if err != nil {
		t.Fatal(err)
	}
	ids, err := LoadPlayerInfo(strings.NewReader("Barnes, David J.\t12345\r\n"))
	if err != nil {
		t.Fatal(err)
	}
	g, r := parse(t, "[White \"Barnes, David J.\"]\n[Black \"Horton, Mark\"]\n[Result \"*\"]\n\n1. e4 *\n")
	if _, err := Apply(g, r, Options{Ratings: ratings, IDs: ids}); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"WhiteElo": "2000", "BlackElo": "1900", "WhiteFideId": "12345"} {
		if got := tag(t, g, name); got != want {
			t.Errorf("%s = %s, want %s", name, got, want)
		}
	}
	if _, ok := g.Tag("BlackFideId"); ok {
		t.Error("BlackFideId added for a player not in the file")
	}
}

// --dropbefore reads only the comments written directly after a move
// (apply.c find_matching_comment on comment_list): a comment that follows
// a NAG belongs to the NAG in the C tool and drops nothing. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 $1 { cmt } 2. Nf3 Nc6 *\n' | pgn-extract --dropbefore cmt --quiet
func TestDropBeforeIgnoresCommentAfterNAG(t *testing.T) {
	g, r := parse(t, "[Result \"*\"]\n\n1. e4 e5 $1 { cmt } 2. Nf3 Nc6 *\n")
	if _, err := Apply(g, r, Options{DropBefore: "cmt"}); err != nil {
		t.Fatal(err)
	}
	if len(g.Moves) != 4 {
		t.Errorf("a comment after a NAG must not match; got %d moves", len(g.Moves))
	}
}
