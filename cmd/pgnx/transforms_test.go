package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The transform flags that take a value or a file, and the combinations
// the C tool refuses, are usage errors (exit 1) rather than silently
// ignored.
func TestTransformFlagErrors(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"no-tags with roster", []string{"convert", "--no-tags", "--seven-tag-roster"}, "--no-tags"},
		{"no-tags with explicit roster", []string{"convert", "--no-tags", "--roster", "Event"}, "--no-tags"},
		{"negative ply limit", []string{"convert", "--ply-limit", "-1"}, "--ply-limit"},
		{"missing player file", []string{"convert", "--add-elo-tags", filepath.Join(t.TempDir(), "none.tsv")}, "none.tsv"},
		{"split needs games", []string{"split"}, "--games"},
		{"split with output", []string{"split", "--games", "2", "-o", "x.pgn"}, "--output"},
		{"split with append", []string{"split", "--games", "2", "--append"}, "--append"},
		{"split with eco-split", []string{"split", "--games", "2", "--eco-split", "1"}, "--eco-split"},
		{"split-variants with no-variations", []string{"convert", "--split-variants", "--no-variations"}, "--no-variations"},
		{"split-depth without split-variants", []string{"convert", "--split-depth", "1"}, "--split-variants"},
		{"negative split-depth", []string{"convert", "--split-variants", "--split-depth", "-1"}, "--split-depth"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, errOut, code := runCLI(t, fools, c.args...)
			if code != 1 {
				t.Fatalf("exit = %d, want 1\n%s", code, errOut)
			}
			if !strings.Contains(errOut, c.want) {
				t.Errorf("stderr %q should name %s", errOut, c.want)
			}
		})
	}
}

// --fen-comments implies --final-fen off, as --fencomments does -F.
func TestFENCommentsImplyNoFinalFEN(t *testing.T) {
	out, _, code := runCLI(t, fools, "convert", "--quiet", "--fen-comments", "--final-fen")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.Contains(out, `{ "`) {
		t.Errorf("quoted final FEN emitted alongside --fen-comments:\n%s", out)
	}
}

// --add-elo-tags and --add-fide-id-tags read pgn-extract's tab-separated
// player files; the oracle for this output is
//
//	pgn-extract --addelotags elo.tsv --addfideidtags id.tsv --quiet
func TestAddPlayerTags(t *testing.T) {
	dir := t.TempDir()
	elo := filepath.Join(dir, "elo.tsv")
	ids := filepath.Join(dir, "id.tsv")
	os.WriteFile(elo, []byte("Barnes, David J.\t2000\nHorton, Mark\t1900\n"), 0o644)
	os.WriteFile(ids, []byte("Barnes, David J.\t12345\n"), 0o644)
	out, errOut, code := runCLI(t, "", "convert", "--quiet", "--add-elo-tags", elo, "--add-fide-id-tags", ids,
		filepath.Join("testdata", "golden", "infiles", "test-plycount.pgn"))
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	want := "[Result \"1/2-1/2\"]\n[WhiteElo \"2000\"]\n[BlackElo \"1900\"]\n[WhiteFideId \"12345\"]\n\n"
	if !strings.Contains(out, want) {
		t.Errorf("output lacks %q:\n%s", want, out)
	}
}

// A game too short for --drop-ply is reported and emitted nowhere; the
// C tool drops it silently (CONFORMANCE.md).
func TestDropPlyTooShortIsReported(t *testing.T) {
	out, errOut, code := runCLI(t, fools+"\n1. e4 e5 2. Nf3 Nc6 3. Bb5 a6 *\n", "convert", "--quiet", "--drop-ply", "5")
	if code != 2 {
		t.Fatalf("exit = %d, want 2\n%s", code, errOut)
	}
	if strings.Contains(out, "Qh4#") || !strings.Contains(out, "a6 *") {
		t.Errorf("short game must go, long game must stay:\n%s", out)
	}
	if !strings.Contains(errOut, "drop") {
		t.Errorf("stderr should say why: %q", errOut)
	}
}

// TotalPlyCount counts variation plies only while variations are kept;
// -Wuci drops them in the C tool, so --total-ply-count -f uci counts the
// main line alone (oracle: pgn-extract --totalplycount -Wuci --quiet).
func TestTotalPlyCountUCI(t *testing.T) {
	const game = "[Result \"*\"]\n\n1. e4 (1. d4 d5) e5 *\n"
	out, _, code := runCLI(t, game, "convert", "--quiet", "--total-ply-count", "-f", "uci")
	if code != 0 || !strings.Contains(out, "[TotalPlyCount \"2\"]") {
		t.Errorf("uci: exit %d\n%s", code, out)
	}
	out, _, code = runCLI(t, game, "convert", "--quiet", "--total-ply-count")
	if code != 0 || !strings.Contains(out, "[TotalPlyCount \"4\"]") {
		t.Errorf("san: exit %d\n%s", code, out)
	}
}

// pgnx split names its files by format, as -# does (.pgn, .epd, .fen,
// .cm; output.c output_file_suffix).
func TestSplitFileSuffix(t *testing.T) {
	for _, format := range []string{"epd", "cm"} {
		t.Run(format, func(t *testing.T) {
			t.Chdir(t.TempDir())
			_, errOut, code := runCLI(t, fools+"\n"+fools, "split", "--quiet", "--games", "1", "-f", format)
			if code != 0 {
				t.Fatalf("exit %d\n%s", code, errOut)
			}
			for _, name := range []string{"1." + format, "2." + format} {
				if _, err := os.Stat(name); err != nil {
					t.Errorf("%s: %v", name, err)
				}
			}
		})
	}
}

// --split-variants emits one game per variation after the main line; the
// run's summary still counts source games, as the C tool's does, and UCI
// output, which drops variations, does not split (argsfile.c: -Wuci
// turns keep_variations off, and output_game splits only with it on).
func TestSplitVariationsCLI(t *testing.T) {
	const src = "[Result \"*\"]\n\n1. e4 e5 (1... c5 2. Nf3 (2. c3)) 2. Nf3 *\n"
	cases := []struct {
		name  string
		args  []string
		games int
	}{
		{"convert", []string{"convert", "--split-variants"}, 3},
		{"depth 1", []string{"convert", "--split-variants", "--split-depth", "1"}, 2},
		{"filter", []string{"filter", "--split-variants", "--player", ""}, 3},
		{"uci", []string{"convert", "--split-variants", "-f", "uci"}, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, errOut, code := runCLI(t, src, c.args...)
			if code != 0 {
				t.Fatalf("exit %d\n%s", code, errOut)
			}
			if n := strings.Count(out, "[Result "); n != c.games {
				t.Errorf("%d games emitted, want %d:\n%s", n, c.games, out)
			}
			if !strings.Contains(errOut, "1 game matched out of 1.") {
				t.Errorf("summary should count the source game:\n%s", errOut)
			}
		})
	}
}

// pgnx split counts source games, not the games they split into: the C
// tool's -#N switches file on num_games_matched (grammar.c
// select_output_file), so one game split into several stays in one file.
func TestSplitFilesCountSourceGames(t *testing.T) {
	t.Chdir(t.TempDir())
	const src = "[Result \"*\"]\n\n1. e4 e5 (1... c5) (1... e6) *\n"
	_, errOut, code := runCLI(t, src+"\n"+src+"\n"+fools, "split", "--quiet", "--games", "2", "--split-variants")
	if code != 0 {
		t.Fatalf("exit %d\n%s", code, errOut)
	}
	for name, games := range map[string]int{"1.pgn": 6, "2.pgn": 1} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if n := strings.Count(string(data), "[Result "); n != games {
			t.Errorf("%s holds %d games, want %d", name, n, games)
		}
	}
	if _, err := os.Stat("3.pgn"); err == nil {
		t.Error("3.pgn should not exist")
	}
}
