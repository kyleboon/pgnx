package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// jsonGames is three games: a mated game with a NAG, a comment and a
// variation, a game from a FEN, and a zero-move game.
const jsonGames = `[Event "One"]
[Result "1-0"]

1. e4 $1 {best} e5 (1... c5) 2. Qh5 Nc6 3. Bc4 Nf6 4. Qxf7# 1-0

[Event "Two"]
[Result "*"]
[SetUp "1"]
[FEN "4k3/8/8/8/8/8/4P3/4K3 b - - 0 40"]

40... Kd7 41. e4 Kd6 42. e5+ *

[Event "Zero"]
[Result "1/2-1/2"]

1/2-1/2
`

// TestConvertJSON is pinned against pgn-extract --json --nomovenumbers
// --notags: one array of game objects on stdout, nothing else.
func TestConvertJSON(t *testing.T) {
	out, errOut, code := runCLI(t, jsonGames, "convert", "--quiet", "--json", "--no-tags")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
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
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestConvertJSONNoGames: no game written means no output at all, as the
// C tool writes "[" only before the first game.
func TestConvertJSONNoGames(t *testing.T) {
	out, _, code := runCLI(t, "", "convert", "--quiet", "--json")
	if code != 0 || out != "" {
		t.Errorf("exit %d, out %q", code, out)
	}
}

// TestFilterJSONDestinations: the matched and unmatched files are each
// one array (where the C tool writes stray commas into the unmatched
// file; CONFORMANCE.md).
func TestFilterJSONDestinations(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pgn")
	matched := filepath.Join(dir, "m.pgn")
	unmatched := filepath.Join(dir, "u.pgn")
	if err := os.WriteFile(in, []byte(jsonGames+jsonGames), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runCLI(t, "", "filter", "--quiet", "--json", "--no-tags", "--tag", `Event "Two"`, "-o", matched, "--unmatched", unmatched, in)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	m, _ := os.ReadFile(matched)
	u, _ := os.ReadFile(unmatched)
	wantM := "[\n{\n\"Moves\":[{ \"move\" : \"Kd7\" }, { \"move\" : \"e4\" }, { \"move\" : \"Kd6\" }, { \"move\" : \"e5+\" }]\n\n},\n{\n\"Moves\":[{ \"move\" : \"Kd7\" }, { \"move\" : \"e4\" }, { \"move\" : \"Kd6\" }, { \"move\" : \"e5+\" }]\n\n}\n]\n"
	if string(m) != wantM {
		t.Errorf("matched:\n%s\nwant:\n%s", m, wantM)
	}
	if n := strings.Count(string(u), "{\n\"Moves\""); n != 4 {
		t.Errorf("unmatched has %d games, want 4:\n%s", n, u)
	}
	if !strings.HasPrefix(string(u), "[\n{\n") || !strings.HasSuffix(string(u), "\n}\n]\n") || strings.Contains(string(u), ",\n,") {
		t.Errorf("unmatched is not one array:\n%s", u)
	}
}

// TestFilterJSONSharedStdout: matched and unmatched games both sent to
// stdout form one array, in input order, since they share one stream.
func TestFilterJSONSharedStdout(t *testing.T) {
	out, errOut, code := runCLI(t, jsonGames, "filter", "--quiet", "--json", "--no-tags", "--tag", `Event "Two"`, "--unmatched", "-")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
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
	if out != want {
		t.Errorf("got:\n%s\nwant:\n%s", out, want)
	}
}

// TestSplitJSON: every numbered file is an array of its own (grammar.c
// select_output_file), pgn-extract --json --nomovenumbers --notags -#2.
func TestSplitJSON(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.pgn")
	if err := os.WriteFile(in, []byte(jsonGames), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	_, errOut, code := runCLI(t, "", "split", "--quiet", "--json", "--no-tags", "--games", "2", in)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	one, _ := os.ReadFile("1.pgn")
	two, _ := os.ReadFile("2.pgn")
	want1 := "[\n{\n\"Moves\":[{ \"move\" : \"e4\", \"nags\" : [\"$1\"]  }, { \"move\" : \"e5\" }, { \"move\" : \"Qh5\" }, { \"move\" : \"Nc6\" }, { \"move\" : \"Bc4\" }, { \"move\" : \"Nf6\" }, { \"move\" : \"Qxf7#\" }]\n\n},\n{\n\"Moves\":[{ \"move\" : \"Kd7\" }, { \"move\" : \"e4\" }, { \"move\" : \"Kd6\" }, { \"move\" : \"e5+\" }]\n\n}\n]\n"
	want2 := "[\n{\n\"Moves\":[]\n\n}\n]\n"
	if string(one) != want1 {
		t.Errorf("1.pgn:\n%s\nwant:\n%s", one, want1)
	}
	if string(two) != want2 {
		t.Errorf("2.pgn:\n%s\nwant:\n%s", two, want2)
	}
}

// TestJSONUsageErrors: the C tool turns --json off with a warning for
// -Wepd and -E; pgnx refuses the combination.
func TestJSONUsageErrors(t *testing.T) {
	cases := [][]string{
		{"convert", "--json", "-f", "epd"},
		{"convert", "--json", "-f", "fen"},
		{"filter", "--json", "-f", "epd"},
		{"convert", "--json", "--eco-split", "1"},
	}
	for _, args := range cases {
		_, errOut, code := runCLI(t, fools, args...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
		if !strings.Contains(errOut, "--json") {
			t.Errorf("%v: stderr %q does not name --json", args, errOut)
		}
	}
}

// TestJSONDropsVariations: --json turns variations off in the C tool
// (main.c), so --split-variants splits nothing and TotalPlyCount counts
// the main line only (pgn-extract --json --nomovenumbers --splitvariants
// --totalplycount).
func TestJSONDropsVariations(t *testing.T) {
	out, errOut, code := runCLI(t, jsonGames, "convert", "--quiet", "--json", "--split-variants", "--total-ply-count")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if n := strings.Count(out, "\"Moves\""); n != 3 {
		t.Errorf("%d games, want 3 (no split):\n%s", n, out)
	}
	if !strings.Contains(out, "\"TotalPlyCount\" : \"7\",\n") {
		t.Errorf("TotalPlyCount should count the 7 main-line plies:\n%s", out)
	}
}

// TestJSONConformance compares pgnx's JSON output with oracle files
// produced by the C tool over the golden corpus inputs. The C repo's
// test/Makefile has no --json target, so these oracles are not vendored
// from it: each was generated with pgn-extract v26-06 (the C binary
// scripts/difftest.sh uses), run in testdata/golden/infiles with the
// command in its row's comment, and --nomovenumbers because the C tool
// otherwise leaks move numbers into its JSON output (CONFORMANCE.md).
func TestJSONConformance(t *testing.T) {
	cases := []struct {
		name   string
		args   []string
		input  string // under testdata/golden/infiles
		oracle string // under testdata/json
	}{
		// pgn-extract --json --nomovenumbers -ofischer-json-out.txt --quiet fischer.pgn
		{"fischer", []string{"convert", "--json"}, "fischer.pgn", "fischer-json-out.txt"},
		// pgn-extract --json --nomovenumbers --plylimit 20 --notags -ofischer-plylimit20-notags-json-out.txt --quiet fischer.pgn
		{"fischer-plylimit20-notags", []string{"convert", "--json", "--ply-limit", "20", "--no-tags"}, "fischer.pgn", "fischer-plylimit20-notags-json-out.txt"},
		// pgn-extract --json --nomovenumbers -Wuci -onajdorf-uci-json-out.txt --quiet najdorf.pgn
		{"najdorf-uci", []string{"convert", "--json", "-f", "uci"}, "najdorf.pgn", "najdorf-uci-json-out.txt"},
		// pgn-extract --json --nomovenumbers -Wxlalg -otest-ucW-xlalg-json-out.txt --quiet test-ucW.pgn
		{"test-ucW-xlalg", []string{"convert", "--json", "-f", "xlalg"}, "test-ucW.pgn", "test-ucW-xlalg-json-out.txt"},
		// pgn-extract --json --nomovenumbers -Welalg -otest-C-elalg-json-out.txt --quiet test-C.pgn
		{"test-C-elalg", []string{"convert", "--json", "-f", "elalg"}, "test-C.pgn", "test-C-elalg-json-out.txt"},
		// pgn-extract --json --nomovenumbers -F --hashcomments --evaluation -otest-C-F-hash-eval-json-out.txt --quiet test-C.pgn
		{"test-C-F-hash-eval", []string{"convert", "--json", "--final-fen", "--hash-comments", "--evaluation"}, "test-C.pgn", "test-C-F-hash-eval-json-out.txt"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			outFile := filepath.Join(t.TempDir(), "out.json")
			args := append(append([]string{}, c.args...), "--quiet", "-o", outFile,
				filepath.Join("testdata", "golden", "infiles", c.input))
			_, errOut, code := runCLI(t, "", args...)
			if code != 0 {
				t.Fatalf("pgnx %v: exit %d\n%s", args, code, errOut)
			}
			got, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "json", c.oracle))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from C oracle %s\n--- first divergence ---\n%s", c.oracle, firstDiff(got, want))
			}
		})
	}
}
