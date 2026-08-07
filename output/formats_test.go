package output

import (
	"bytes"
	"strings"
	"testing"
)

func render(t *testing.T, src string, opts Options) string {
	t.Helper()
	g, r := gameAndReplay(t, src)
	var buf bytes.Buffer
	if err := NewWriter(&buf, opts).WriteGame(g, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

const scandi = "[Result \"*\"]\n\n1. e4 d5 2. exd5 Qxd5 *\n"

// TestFormats pins the long-algebraic family's movetext rendering against
// the C oracle. Oracle invocations (from ~/code/pgn-extract):
//
//	printf '[Result "*"]\n\n1. e4 d5 2. exd5 Qxd5 *\n' | ./build/bin/pgn-extract -Wlalg --quiet
//	  -> 1. e2e4 d7d5 2. e4d5 d8d5 *
//	printf '[Result "*"]\n\n1. e4 d5 2. exd5 Qxd5 *\n' | ./build/bin/pgn-extract -Whalg --quiet
//	  -> 1. e2-e4 d7-d5 2. e4-d5 d8-d5 *
//	printf '[Result "*"]\n\n1. e4 d5 2. exd5 Qxd5 *\n' | ./build/bin/pgn-extract -Welalg --quiet
//	  -> 1. e2e4 d7d5 2. e4d5 Qd8d5 *
//	printf '[Result "*"]\n\n1. e4 d5 2. exd5 Qxd5 *\n' | ./build/bin/pgn-extract -Wxlalg --quiet
//	  -> 1. e2-e4 d7-d5 2. e4xd5 Qd8xd5 *
func TestFormats(t *testing.T) {
	cases := []struct {
		f    Format
		want string // movetext line
	}{
		{LAlg, "1. e2e4 d7d5 2. e4d5 d8d5 *"},
		{HAlg, "1. e2-e4 d7-d5 2. e4-d5 d8-d5 *"},
		{ELAlg, "1. e2e4 d7d5 2. e4d5 Qd8d5 *"},
		{XLAlg, "1. e2-e4 d7-d5 2. e4xd5 Qd8xd5 *"},
	}
	for _, c := range cases {
		got := render(t, scandi, Options{Format: c.f})
		if !strings.Contains(got, c.want) {
			t.Errorf("format %v:\n%s\nwant line %q", c.f, got, c.want)
		}
	}
}

// TestChecksKeptInAlgebraicFamily pins that LAlg/HAlg/ELAlg/XLAlg keep check
// suffixes (unlike UCI, which strips them). Oracle:
//
//	printf '[Result "*"]\n\n1. f3 e5 2. g4 Qh4# *\n' | ./build/bin/pgn-extract -Wlalg --quiet
//	  -> 1. f2f3 e7e5 2. g2g4 d8h4#
//	printf '[Result "*"]\n\n1. f3 e5 2. g4 Qh4# *\n' | ./build/bin/pgn-extract -Whalg --quiet
//	  -> 1. f2-f3 e7-e5 2. g2-g4 d8-h4#
func TestChecksKeptInAlgebraicFamily(t *testing.T) {
	src := "[Result \"*\"]\n\n1. f3 e5 2. g4 Qh4# *\n"
	cases := []struct {
		f    Format
		want string
	}{
		{LAlg, "1. f2f3 e7e5 2. g2g4 d8h4#"},
		{HAlg, "1. f2-f3 e7-e5 2. g2-g4 d8-h4#"},
	}
	for _, c := range cases {
		got := render(t, src, Options{Format: c.f})
		if !strings.Contains(got, c.want) {
			t.Errorf("format %v:\n%s\nwant line %q", c.f, got, c.want)
		}
	}
}

// TestEnPassantSuffix pins that ELAlg/XLAlg append "ep" to an en passant
// capture, while LAlg/HAlg/UCI show a plain from-to square pair. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 a6 2. e5 f5 3. exf6 *\n' | ./build/bin/pgn-extract -Wlalg --quiet
//	  -> 3. e5f6
//	printf '[Result "*"]\n\n1. e4 a6 2. e5 f5 3. exf6 *\n' | ./build/bin/pgn-extract -Whalg --quiet
//	  -> 3. e5-f6
//	printf '[Result "*"]\n\n1. e4 a6 2. e5 f5 3. exf6 *\n' | ./build/bin/pgn-extract -Welalg --quiet
//	  -> 3. e5f6ep
//	printf '[Result "*"]\n\n1. e4 a6 2. e5 f5 3. exf6 *\n' | ./build/bin/pgn-extract -Wxlalg --quiet
//	  -> 3. e5xf6ep
func TestEnPassantSuffix(t *testing.T) {
	src := "[Result \"*\"]\n\n1. e4 a6 2. e5 f5 3. exf6 *\n"
	cases := []struct {
		f    Format
		want string
	}{
		{LAlg, "3. e5f6 *"},
		{HAlg, "3. e5-f6 *"},
		{ELAlg, "3. e5f6ep *"},
		{XLAlg, "3. e5xf6ep *"},
	}
	for _, c := range cases {
		got := render(t, src, Options{Format: c.f})
		if !strings.Contains(got, c.want) {
			t.Errorf("format %v:\n%s\nwant line %q", c.f, got, c.want)
		}
	}
}

// TestPromotionUppercase pins that promotion piece letters are uppercase
// (the C oracle's default output_piece_characters), for every algebraic
// family format including UCI. Oracle:
//
//	printf '[Result "*"]\n\n1. a4 h5 2. a5 h4 3. a6 h3 4. axb7 hxg2 5. bxa8=Q gxh1=Q *\n' \
//	  | ./build/bin/pgn-extract -Wlalg --quiet
//	  -> 5. b7a8Q g2h1Q
//	  (same, hyphenated) -Whalg  -> 5. b7-a8Q g2-h1Q
//	  (same)            -Wxlalg -> 5. b7xa8Q g2xh1Q
//	  (same)            -Wuci   -> b7a8Q g2h1Q
func TestPromotionUppercase(t *testing.T) {
	src := "[Result \"*\"]\n\n1. a4 h5 2. a5 h4 3. a6 h3 4. axb7 hxg2 5. bxa8=Q gxh1=Q *\n"
	cases := []struct {
		f    Format
		want string
	}{
		{LAlg, "5. b7a8Q g2h1Q *"},
		{HAlg, "5. b7-a8Q g2-h1Q *"},
		{XLAlg, "5. b7xa8Q g2xh1Q *"},
	}
	for _, c := range cases {
		// A wide LineWidth keeps the whole movetext on one line regardless
		// of format-specific token length (hyphens/x add width), so the
		// wanted substring can't be split across a wrap point.
		got := render(t, src, Options{Format: c.f, LineWidth: 200})
		if !strings.Contains(got, c.want) {
			t.Errorf("format %v:\n%s\nwant line %q", c.f, got, c.want)
		}
	}

	uci := render(t, src, Options{Format: UCI, LineWidth: 200})
	body := uci[strings.Index(uci, "\n\n")+2:]
	if !strings.Contains(body, "b7a8Q g2h1Q") {
		t.Errorf("uci promotion: %q", body)
	}
}

// TestCastlingAsKingMove pins that castling renders as a plain king move
// (not O-O) in every algebraic-family format, with no piece-letter prefix
// and (for HAlg/XLAlg) a hyphen separator, never "x". Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bc4 Bc5 4. O-O *\n' | ./build/bin/pgn-extract -Wlalg --quiet
//	  -> 4. e1g1
//	  -Whalg  -> 4. e1-g1
//	  -Welalg -> 4. e1g1
//	  -Wxlalg -> 4. e1-g1
func TestCastlingAsKingMove(t *testing.T) {
	src := "[Result \"*\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bc4 Bc5 4. O-O *\n"
	cases := []struct {
		f    Format
		want string
	}{
		{LAlg, "4. e1g1 *"},
		{HAlg, "4. e1-g1 *"},
		{ELAlg, "4. e1g1 *"},
		{XLAlg, "4. e1-g1 *"},
	}
	for _, c := range cases {
		got := render(t, src, Options{Format: c.f})
		if !strings.Contains(got, c.want) {
			t.Errorf("format %v:\n%s\nwant line %q", c.f, got, c.want)
		}
	}
}

// TestNonCastlingKingMoveGetsPiecePrefix pins that an ordinary (non-castling)
// king move DOES get the "K" piece-letter prefix in ELAlg/XLAlg — unlike
// castling, which never does. This distinguishes "is this a king move" from
// the C oracle's actual signal (move class PIECE_MOVE vs KINGSIDE_CASTLE/
// QUEENSIDE_CASTLE). Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 2. Ke2 Ke7 *\n' | ./build/bin/pgn-extract -Welalg --quiet
//	  -> 1. e2e4 e7e5 2. Ke1e2 Ke8e7 *
//	printf '[Result "*"]\n\n1. e4 e5 2. Ke2 Ke7 *\n' | ./build/bin/pgn-extract -Wxlalg --quiet
//	  -> 1. e2-e4 e7-e5 2. Ke1-e2 Ke8-e7 *
func TestNonCastlingKingMoveGetsPiecePrefix(t *testing.T) {
	src := "[Result \"*\"]\n\n1. e4 e5 2. Ke2 Ke7 *\n"
	cases := []struct {
		f    Format
		want string
	}{
		{ELAlg, "2. Ke1e2 Ke8e7 *"},
		{XLAlg, "2. Ke1-e2 Ke8-e7 *"},
	}
	for _, c := range cases {
		got := render(t, src, Options{Format: c.f})
		if !strings.Contains(got, c.want) {
			t.Errorf("format %v:\n%s\nwant line %q", c.f, got, c.want)
		}
	}
}

// TestUCIStripsEverything pins that UCI output strips move numbers,
// comments, NAGs, variations, and check symbols. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 {c} e5 (1... c5) 2. Qh5 Nc6 3. Qxf7# *\n' | ./build/bin/pgn-extract -Wuci --quiet
//	  -> e2e4 e7e5 d1h5 b8c6 h5f7 *
func TestUCIStripsEverything(t *testing.T) {
	out := render(t, "[Result \"*\"]\n\n1. e4 {c} e5 (1... c5) 2. Qh5 Nc6 3. Qxf7# *\n", Options{Format: UCI})
	body := out[strings.Index(out, "\n\n")+2:]
	if strings.ContainsAny(body, "{}()+#.") {
		t.Errorf("UCI body must be bare moves: %q", body)
	}
	if !strings.Contains(body, "h5f7") {
		t.Errorf("missing uci move: %q", body)
	}
	want := "e2e4 e7e5 d1h5 b8c6 h5f7 *"
	if strings.TrimSpace(body) != want {
		t.Errorf("uci body = %q, want %q", strings.TrimSpace(body), want)
	}
}

// TestUCICastling pins that UCI renders castling as a plain king move.
// Oracle:
//
//	printf '[Result "*"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bc4 Bc5 4. O-O *\n' | ./build/bin/pgn-extract -Wuci --quiet
//	  -> e2e4 e7e5 g1f3 b8c6 f1c4 f8c5 e1g1 *
func TestUCICastling(t *testing.T) {
	out := render(t, "[Result \"*\"]\n\n1. e4 e5 2. Nf3 Nc6 3. Bc4 Bc5 4. O-O *\n", Options{Format: UCI})
	body := strings.TrimSpace(out[strings.Index(out, "\n\n")+2:])
	want := "e2e4 e7e5 g1f3 b8c6 f1c4 f8c5 e1g1 *"
	if body != want {
		t.Errorf("uci body = %q, want %q", body, want)
	}
}

// TestEPDPerPosition pins EPD's per-position line shape: no tag section,
// one line per position (including the initial), each
// "<placement> <side> <castling> <ep> c0 <White>-<Black> <Event> <Site>
// <Date>; c1 <Result>;". Oracle:
//
//	printf '[Event "E"]\n[White "W"]\n[Black "B"]\n[Result "*"]\n\n1. e4 *\n' | ./build/bin/pgn-extract -Wepd --quiet
//	  -> rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - c0 W-B E; c1 *;
//	     rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 c0 W-B E; c1 *;
func TestEPDPerPosition(t *testing.T) {
	out := render(t, "[Event \"E\"]\n[White \"W\"]\n[Black \"B\"]\n[Result \"*\"]\n\n1. e4 *\n", Options{Format: EPD})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 { // initial + after 1.e4
		t.Fatalf("EPD lines = %d:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq -") ||
		!strings.Contains(lines[0], "c0 W-B") || !strings.HasSuffix(lines[0], "c1 *;") {
		t.Errorf("EPD line 0 = %q", lines[0])
	}
	want0 := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - c0 W-B E; c1 *;"
	want1 := "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 c0 W-B E; c1 *;"
	if lines[0] != want0 {
		t.Errorf("EPD line 0 = %q, want %q", lines[0], want0)
	}
	if lines[1] != want1 {
		t.Errorf("EPD line 1 = %q, want %q", lines[1], want1)
	}
}

// TestEPDMissingTags pins that EPD's c0 comment omits missing tags outright
// (no "?" defaulting like the SAN/FEN tag section), leaving White/Black as a
// bare "-" separator when both are absent. Oracle:
//
//	printf '[Result "*"]\n\n1. e4 *\n' | ./build/bin/pgn-extract -Wepd --quiet
//	  -> rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - c0 -; c1 *;
//	     rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 c0 -; c1 *;
func TestEPDMissingTags(t *testing.T) {
	out := render(t, "[Result \"*\"]\n\n1. e4 *\n", Options{Format: EPD})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	want0 := "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - c0 -; c1 *;"
	want1 := "rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 c0 -; c1 *;"
	if len(lines) != 2 || lines[0] != want0 || lines[1] != want1 {
		t.Errorf("EPD lines = %q, want [%q %q]", lines, want0, want1)
	}
}

// TestFENGame pins the FEN format's structure: the same tag section as
// SAN/Source (default ALL_TAGS: seven roster plus well-known/extra tags, per
// output.c:print_FEN_game which calls show_tags when tag_output_format is
// its default ALL_TAGS), a blank line, then one full 6-field FEN string per
// position (including the initial), and a trailing blank line. There is no
// golden oracle target for FEN (per the task brief); this is pinned by
// reading output.c:print_FEN_game / print_FEN_move_list / apply.c:
// get_FEN_string directly, and cross-checked against the EPD oracle's
// placement/side/castling/ep fields (build_basic_EPD_string, shared with
// FEN) plus "0 1"-style half-move/full-move suffixes. Manually verified
// against the real binary for the tag-section framing:
//
//	printf '[Event "E"]\n[White "W"]\n[Black "B"]\n[Result "*"]\n\n1. e4 *\n' | ./build/bin/pgn-extract -Wfen --quiet
//	  -> [Event "E"]
//	     [Site "?"]
//	     [Date "????.??.??"]
//	     [Round "?"]
//	     [White "W"]
//	     [Black "B"]
//	     [Result "*"]
//
//	     rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1
//	     rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1
func TestFENGame(t *testing.T) {
	out := render(t, "[Event \"E\"]\n[White \"W\"]\n[Black \"B\"]\n[Result \"*\"]\n\n1. e4 *\n", Options{Format: FEN})
	want := "[Event \"E\"]\n[Site \"?\"]\n[Date \"????.??.??\"]\n[Round \"?\"]\n[White \"W\"]\n[Black \"B\"]\n[Result \"*\"]\n\n" +
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1\n" +
		"rnbqkbnr/pppppppp/8/8/4P3/8/PPPP1PPP/RNBQKBNR b KQkq e3 0 1\n\n"
	if out != want {
		t.Errorf("got:\n%q\nwant:\n%q", out, want)
	}
}

// TestFENGameNoResultLine pins that FEN output has no trailing result
// token — just FEN lines, unlike SAN/algebraic formats.
func TestFENGameNoResultLine(t *testing.T) {
	out := render(t, "[Result \"*\"]\n\n1. e4 e5 *\n", Options{Format: FEN})
	if !strings.HasSuffix(out, "\n\n") {
		t.Fatalf("expected trailing blank line, got:\n%q", out)
	}
	// The only "*" that may appear anywhere is inside the Result tag value
	// in the tag section — the FEN body itself must not contain one.
	_, body, ok := strings.Cut(out, "\n\n")
	if !ok {
		t.Fatalf("no blank line separating tags from FEN body:\n%s", out)
	}
	if strings.Contains(body, "*") {
		t.Errorf("FEN body should not contain a result token:\n%q", body)
	}
}
