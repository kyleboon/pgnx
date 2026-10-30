package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const twoGames = "[White \"Fischer\"]\n[Black \"Spassky\"]\n\n1. e4 e5 *\n\n[White \"Petrosian\"]\n[Black \"Tal\"]\n\n1. d4 d5 *\n"

func TestFilterNoCriteriaMatchesConvert(t *testing.T) {
	want, _, code := runCLI(t, twoGames, "convert", "--quiet")
	if code != 0 {
		t.Fatalf("convert exit %d", code)
	}
	got, errOut, code := runCLI(t, twoGames, "filter")
	if code != 0 {
		t.Fatalf("filter exit %d: %s", code, errOut)
	}
	if got != want {
		t.Errorf("filter without criteria differs from convert:\n%s\n---\n%s", got, want)
	}
	if !strings.Contains(errOut, "2 games matched out of 2.") {
		t.Errorf("summary: %q", errOut)
	}
}

func TestFilterPlayerRoutesMatchedAndUnmatched(t *testing.T) {
	dir := t.TempDir()
	matched := filepath.Join(dir, "m.pgn")
	unmatched := filepath.Join(dir, "u.pgn")
	_, errOut, code := runCLI(t, twoGames, "filter", "--player", "Fischer", "-o", matched, "--unmatched", unmatched)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	m, _ := os.ReadFile(matched)
	u, _ := os.ReadFile(unmatched)
	if !strings.Contains(string(m), "Fischer") || strings.Contains(string(m), "Petrosian") {
		t.Errorf("matched: %s", m)
	}
	if !strings.Contains(string(u), "Petrosian") || strings.Contains(string(u), "Fischer") {
		t.Errorf("unmatched: %s", u)
	}
	if !strings.Contains(errOut, "1 game matched out of 2.") {
		t.Errorf("summary: %q", errOut)
	}
}

func TestFilterUnmatchedToStdout(t *testing.T) {
	out, _, code := runCLI(t, twoGames, "filter", "--quiet", "--player", "Fischer", "--suppress-matched", "--unmatched", "-")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(out, "Petrosian") || strings.Contains(out, "Fischer") {
		t.Errorf("stdout: %s", out)
	}
}

func TestFilterSuppressMatchedLeavesStdoutEmpty(t *testing.T) {
	out, errOut, code := runCLI(t, twoGames, "filter", "--player", "Fischer", "--suppress-matched")
	if code != 0 || out != "" {
		t.Errorf("exit %d stdout %q", code, out)
	}
	if strings.Contains(errOut, "matched out of") {
		t.Errorf("summary should be suppressed: %q", errOut)
	}
}

func TestFilterStopAfterStopsReading(t *testing.T) {
	// The second game is unparsable (unterminated comment); --stop-after 1
	// must stop before the scanner reaches it.
	in := "[White \"Fischer\"]\n\n1. e4 *\n\n[White \"Fischer\"]\n\n1. e4 {never closed\n"
	out, errOut, code := runCLI(t, in, "filter", "--quiet", "--player", "Fischer", "--stop-after", "1")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Count(out, "[White") != 1 {
		t.Errorf("want exactly one game: %s", out)
	}
	_, _, code = runCLI(t, in, "filter", "--quiet", "--player", "Fischer")
	if code != 2 {
		t.Errorf("without --stop-after the bad game is reached: exit %d", code)
	}
}

func TestFilterStopAfterAcrossFiles(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.pgn")
	b := filepath.Join(dir, "b.pgn")
	os.WriteFile(a, []byte("[White \"Fischer\"]\n\n1. e4 *\n"), 0o644)
	os.WriteFile(b, []byte("[White \"Fischer\"]\n\n1. d4 *\n"), 0o644)
	out, _, code := runCLI(t, "", "filter", "--quiet", "--stop-after", "1", a, b)
	if code != 0 || strings.Contains(out, "d4") {
		t.Errorf("second file should not be read: exit %d out %s", code, out)
	}
}

func TestFilterConflictingFlags(t *testing.T) {
	for _, args := range [][]string{
		{"--checkmate", "--stalemate"},
		{"--fifty", "--seventy-five"},
		{"--higher-rated-winner", "--lower-rated-winner"},
		{"--no-setup-tags", "--only-setup-tags"},
		{"--moves", "0"},
		{"--min-moves", "10", "--max-moves", "5"},
		{"--select-only", "3,2"},
		{"--tag", "White Fischer"},
		{"--material", "kx k"},
		{"--repetition=-1"},
		{"--first-game", "5", "--game-limit", "2"},
	} {
		_, errOut, code := runCLI(t, fools, append([]string{"filter"}, args...)...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1 (%s)", args, code, errOut)
		}
	}
}

func TestFilterNoBadResultsExit2(t *testing.T) {
	in := "[Result \"1-0\"]\n\n1. e4 0-1\n\n[Result \"1-0\"]\n\n1. e4 1-0\n"
	out, errOut, code := runCLI(t, in, "filter", "--quiet", "--no-bad-results")
	if code != 2 {
		t.Fatalf("exit %d", code)
	}
	if strings.Count(out, "[Result") != 1 || !strings.Contains(errOut, "game 1") {
		t.Errorf("out %q err %q", out, errOut)
	}
}

func TestFilterTagFileMissing(t *testing.T) {
	_, errOut, code := runCLI(t, fools, "filter", "--tag-file", filepath.Join(t.TempDir(), "nope.txt"))
	if code != 1 || !strings.Contains(errOut, "nope.txt") {
		t.Errorf("exit %d err %q", code, errOut)
	}
}

func TestFilterTagFileAndRepetitionFlag(t *testing.T) {
	dir := t.TempDir()
	tf := filepath.Join(dir, "t.txt")
	os.WriteFile(tf, []byte("% criteria\nWhite \"Fischer\"\r\n"), 0o644)
	out, _, code := runCLI(t, twoGames, "filter", "--quiet", "--tag-file", tf)
	if code != 0 || strings.Contains(out, "Petrosian") || !strings.Contains(out, "Fischer") {
		t.Errorf("tag-file: exit %d out %s", code, out)
	}

	rep := "1. Nf3 Nf6 2. Ng1 Ng8 3. Nf3 Nf6 4. Ng1 Ng8 *\n"
	out, _, code = runCLI(t, rep, "filter", "--quiet", "--repetition", "--mark-matches", "R")
	if code != 0 || !strings.Contains(out, "{ R }") {
		t.Errorf("--repetition: exit %d out %s", code, out)
	}
	out, _, code = runCLI(t, rep, "filter", "--quiet", "--repetition=5")
	if code != 0 || out != "" {
		t.Errorf("--repetition=5: exit %d out %q", code, out)
	}
}

func TestFilterKeepBrokenEvaluated(t *testing.T) {
	in := "[White \"Fischer\"]\n\n1. e4 e4 *\n"
	out, _, code := runCLI(t, in, "filter", "--quiet", "--keep-broken", "--player", "Fischer")
	if code != 2 || !strings.Contains(out, "e4 e4") {
		t.Errorf("kept broken game: exit %d out %q", code, out)
	}
	out, _, code = runCLI(t, in, "filter", "--quiet", "--keep-broken", "--player", "Tal", "--unmatched", "-")
	if code != 2 || !strings.Contains(out, "e4 e4") {
		t.Errorf("kept broken game to unmatched: exit %d out %q", code, out)
	}
}

func TestFilterUsageErrorsPrinted(t *testing.T) {
	_, errOut, code := runCLI(t, fools, "filter", "--checkmate", "--stalemate")
	if code != 1 || !strings.Contains(errOut, "checkmate") {
		t.Errorf("exit %d err %q", code, errOut)
	}
}

const sicilianAndQueens = "[White \"Sicilian\"]\n\n1. e4 c5 2. Nf3 d6 *\n\n[White \"Queens\"]\n\n1. d4 d5 2. c4 e6 *\n"

func TestFilterPositionLine(t *testing.T) {
	out, errOut, code := runCLI(t, sicilianAndQueens, "filter", "--quiet", "--position", "e4 c5", "--unmatched", "-", "--suppress-matched")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "Queens") || strings.Contains(out, "Sicilian") {
		t.Errorf("unmatched: %s", out)
	}
	out, _, _ = runCLI(t, sicilianAndQueens, "filter", "--quiet", "--position", "1. e4 c5 *")
	if !strings.Contains(out, "Sicilian") || strings.Contains(out, "Queens") {
		t.Errorf("matched: %s", out)
	}
	// Transposition: the position is reached by a different move order.
	out, _, _ = runCLI(t, "1. Nf3 d6 2. e4 c5 *\n", "filter", "--quiet", "--position", "e4 c5 Nf3 d6")
	if !strings.Contains(out, "Nf3 d6") {
		t.Errorf("transposition: %s", out)
	}
}

func TestFilterPositionFile(t *testing.T) {
	dir := t.TempDir()
	pf := filepath.Join(dir, "x.txt")
	os.WriteFile(pf, []byte("% positions\n\nd4 d5 c4\n"), 0o644)
	out, _, code := runCLI(t, sicilianAndQueens, "filter", "--quiet", "--position-file", pf)
	if code != 0 || !strings.Contains(out, "Queens") || strings.Contains(out, "Sicilian") {
		t.Errorf("exit %d out %s", code, out)
	}
	os.WriteFile(pf, []byte("e4 e5\ne4 Qh5\n"), 0o644)
	_, errOut, code := runCLI(t, sicilianAndQueens, "filter", "--position-file", pf)
	if code != 1 || !strings.Contains(errOut, "x.txt") || !strings.Contains(errOut, "Qh5") {
		t.Errorf("unreplayable line: exit %d err %q", code, errOut)
	}
}

func TestFilterFENAndHash(t *testing.T) {
	afterE4C5 := "rnbqkbnr/pp1ppppp/8/2p5/4P3/8/PPPP1PPP/RNBQKBNR w KQkq c6 0 2"
	out, _, code := runCLI(t, sicilianAndQueens, "filter", "--quiet", "--fen", afterE4C5)
	if code != 0 || !strings.Contains(out, "Sicilian") || strings.Contains(out, "Queens") {
		t.Errorf("--fen: exit %d out %s", code, out)
	}
	out, _, code = runCLI(t, sicilianAndQueens, "filter", "--quiet", "--tag", `FEN "`+afterE4C5+`"`)
	if code != 0 || !strings.Contains(out, "Sicilian") || strings.Contains(out, "Queens") {
		t.Errorf("--tag FEN: exit %d out %s", code, out)
	}
	// Polyglot hash of the start position: every game reaches it, but
	// --black-to-move rules it out.
	out, _, code = runCLI(t, sicilianAndQueens, "filter", "--quiet", "--hash", "463b96181691fc9c")
	if code != 0 || strings.Count(out, "[White") != 2 {
		t.Errorf("--hash: exit %d out %s", code, out)
	}
	out, _, code = runCLI(t, sicilianAndQueens, "filter", "--quiet", "--hash", "463b96181691fc9c", "--black-to-move")
	if code != 0 || out != "" {
		t.Errorf("--hash --black-to-move: exit %d out %s", code, out)
	}
}

func TestFilterFENPatternMarks(t *testing.T) {
	out, errOut, code := runCLI(t, sicilianAndQueens, "filter", "--quiet", "--fen-pattern", "*/*/*/*/????P???/*/*/*", "--mark-matches", "X")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "1. e4 { X } 1... c5") || strings.Contains(out, "Queens") {
		t.Errorf("mark: %s", out)
	}
	// --fen-pattern-i also matches the colour-reversed pattern: a White
	// pawn on d4, or a Black pawn on d5.
	in := sicilianAndQueens + "\n[White \"Scandinavian\"]\n\n1. e4 d5 *\n"
	out, _, _ = runCLI(t, in, "filter", "--quiet", "--fen-pattern-i", "*/*/*/*/???P????/*/*/*", "--mark-matches", "X")
	if strings.Contains(out, "Sicilian") || !strings.Contains(out, "1. d4 { X } 1... d5") || !strings.Contains(out, "1. e4 d5 { X }") {
		t.Errorf("inverse mark: %s", out)
	}
	out, _, _ = runCLI(t, in, "filter", "--quiet", "--fen-pattern", "*/*/*/*/???P????/*/*/*")
	if strings.Contains(out, "Scandinavian") || !strings.Contains(out, "Queens") {
		t.Errorf("plain pattern is not colour-reversed: %s", out)
	}
}

func TestFilterTagFilePatternLabel(t *testing.T) {
	dir := t.TempDir()
	tf := filepath.Join(dir, "t.txt")
	os.WriteFile(tf, []byte("% patterns\nFENPattern \"*/*/*/*/????P???/*/*/*\" KingPawn\nFENPatternI \"*/*/*/*/???P????/*/*/*\" QueenPawn\n"), 0o644)
	out, errOut, code := runCLI(t, sicilianAndQueens, "filter", "--quiet", "--tag-file", tf, "--add-label-tag")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "[MatchLabel \"KingPawn\"]") || !strings.Contains(out, "[MatchLabel \"QueenPawn\"]") {
		t.Errorf("labels: %s", out)
	}
	out, _, _ = runCLI(t, sicilianAndQueens, "filter", "--quiet", "--tag-file", tf)
	if strings.Contains(out, "MatchLabel") {
		t.Errorf("no label tag without --add-label-tag: %s", out)
	}
	out, _, _ = runCLI(t, sicilianAndQueens, "filter", "--quiet", "--tag", `FENPattern "*/*/*/*/????P???/*/*/*" Lbl`, "--add-label-tag")
	if !strings.Contains(out, "[MatchLabel \"Lbl\"]") || strings.Contains(out, "Queens") {
		t.Errorf("--tag pattern: %s", out)
	}
}

func TestFilterStartPlyAndMatchPlyLimit(t *testing.T) {
	// The pattern matches after 1. e4 (ply 1); starting at ply 2 misses it
	// in the Sicilian but the e-pawn is still there at later plies.
	pat := "*/*/*/*/????P???/*/*/*"
	out, _, code := runCLI(t, "1. e4 e5 2. Nf3 *\n", "filter", "--quiet", "--fen-pattern", pat, "--start-ply", "2", "--mark-matches", "X")
	if code != 0 || !strings.Contains(out, "e5 { X }") {
		t.Errorf("start-ply: exit %d out %s", code, out)
	}
	out, _, code = runCLI(t, "1. d4 d5 2. e4 *\n", "filter", "--quiet", "--fen-pattern", pat, "--match-ply-limit", "2")
	if code != 0 || out != "" {
		t.Errorf("match-ply-limit cuts the search: exit %d out %s", code, out)
	}
	out, _, code = runCLI(t, "1. d4 d5 2. e4 *\n", "filter", "--quiet", "--fen-pattern", pat, "--match-ply-limit", "3")
	if code != 0 || out == "" {
		t.Errorf("match-ply-limit reaches the move: exit %d out %s", code, out)
	}
}

func TestFilterPositionalUsageErrors(t *testing.T) {
	for _, args := range [][]string{
		{"--hash", "xyz"},
		{"--hash", "0123456789abcdef0"},
		{"--position", "e4 c5 Nf3 d6 d4 cxd4", "--match-ply-limit", "5"},
		{"--match-ply-limit", "5", "--position", "e4 c5 Nf3 d6 d4 cxd4"},
		{"--match-ply-limit", "0"},
		{"--white-to-move", "--black-to-move"},
		{"--start-ply", "0"},
		{"--position", "e4 e4"},
		{"--fen", "not a fen"},
		{"--fen-pattern", "*/*/*"},
		{"--sequence", "%"},
	} {
		_, errOut, code := runCLI(t, fools, append([]string{"filter"}, args...)...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1 (%s)", args, code, errOut)
		}
	}
	_, errOut, code := runCLI(t, fools, "filter", "--position", "e4 c5 Nf3 d6 d4 cxd4", "--match-ply-limit", "5")
	if code != 1 || !strings.Contains(errOut, "5") || !strings.Contains(errOut, "14") {
		t.Errorf("limit error must quote both numbers: %q", errOut)
	}
	// --exact-order / --sequence-anywhere without sequences are no-ops.
	out, _, code := runCLI(t, fools, "filter", "--quiet", "--exact-order", "--sequence-anywhere")
	if code != 0 || !strings.Contains(out, "Qh4#") {
		t.Errorf("no-op flags: exit %d out %s", code, out)
	}
}

func TestFilterSequences(t *testing.T) {
	in := "[White \"Straight\"]\n\n1. d4 Nf6 2. Nf3 e6 *\n\n[White \"Permuted\"]\n\n1. Nf3 Nf6 2. d4 e6 *\n\n[White \"Other\"]\n\n1. e4 e5 *\n"
	out, errOut, code := runCLI(t, in, "filter", "--quiet", "--sequence", "d4 Nf6 Nf3")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "Straight") || !strings.Contains(out, "Permuted") || strings.Contains(out, "Other") {
		t.Errorf("permutation: %s", out)
	}
	out, _, _ = runCLI(t, in, "filter", "--quiet", "--sequence", "d4 Nf6 Nf3", "--exact-order")
	if !strings.Contains(out, "Straight") || strings.Contains(out, "Permuted") {
		t.Errorf("exact order: %s", out)
	}
	out, _, _ = runCLI(t, in, "filter", "--quiet", "--sequence", "Nf3 e6", "--exact-order")
	if out != "" {
		t.Errorf("sequence from the first move only: %s", out)
	}
	out, _, _ = runCLI(t, in, "filter", "--quiet", "--sequence", "Nf3 e6", "--exact-order", "--sequence-anywhere")
	if !strings.Contains(out, "Straight") || strings.Contains(out, "Permuted") {
		t.Errorf("anywhere: %s", out)
	}

	dir := t.TempDir()
	sf := filepath.Join(dir, "v.txt")
	os.WriteFile(sf, []byte("% sequences\n\ne4 e5\nd4 * Nf3\n"), 0o644)
	out, _, code = runCLI(t, in, "filter", "--quiet", "--sequence-file", sf, "--exact-order")
	if code != 0 || !strings.Contains(out, "Straight") || !strings.Contains(out, "Other") || strings.Contains(out, "Permuted") {
		t.Errorf("sequence file: exit %d out %s", code, out)
	}
}
