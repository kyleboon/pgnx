package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

// Each row mirrors a target in the C repo's test/Makefile (test/Makefile in
// https://github.com/dbarnesuk/pgn-extract, vendored here as
// testdata/golden). The C command is kept as a comment on each row so drift
// between this table and the oracle is auditable: if pgn-extract's Makefile
// changes a flag or an oracle file, the comment and the row will visibly
// disagree.
//
// Rows are restricted to convert-equivalent behavior: pure move/tag output
// formatting with no game selection, filtering, deduplication, or ECO
// classification. See the "Phase 2+" comment at the bottom of this table
// for targets deliberately excluded on that basis.
var goldenCases = []struct {
	name   string
	args   []string // pgnx args; input/output paths are appended by the runner
	input  string   // file under testdata/golden/infiles
	oracle string   // file under testdata/golden/outfiles
}{
	// test-7: pgn-extract -7 -otest-7-out.pgn --quiet test-7.pgn
	{"test-7", []string{"convert", "--seven-tag-roster"}, "test-7.pgn", "test-7-out.pgn"},
	// test-NN: pgn-extract -N -otest-NN-out.pgn --quiet test-N.pgn
	{"test-NN", []string{"convert", "--no-nags"}, "test-N.pgn", "test-NN-out.pgn"},
	// test-VV: pgn-extract -V -otest-VV-out.pgn --quiet test-V.pgn
	{"test-VV", []string{"convert", "--no-variations"}, "test-V.pgn", "test-VV-out.pgn"},
	// test-WW: pgn-extract -Whalg -otest-WWhalg-out.pgn --quiet test-ucW.pgn
	{"test-WWhalg", []string{"convert", "-f", "halg"}, "test-ucW.pgn", "test-WWhalg-out.pgn"},
	// test-WW: pgn-extract -Wlalg -otest-WWlalg-out.pgn --quiet test-ucW.pgn
	{"test-WWlalg", []string{"convert", "-f", "lalg"}, "test-ucW.pgn", "test-WWlalg-out.pgn"},
	// test-WW: pgn-extract -Welalg -otest-WWelalg-out.pgn --quiet test-ucW.pgn
	{"test-WWelalg", []string{"convert", "-f", "elalg"}, "test-ucW.pgn", "test-WWelalg-out.pgn"},
	// test-WW: pgn-extract -Wxlalg -otest-WWxlalg-out.pgn --quiet test-ucW.pgn
	{"test-WWxlalg", []string{"convert", "-f", "xlalg"}, "test-ucW.pgn", "test-WWxlalg-out.pgn"},
	// test-WW: pgn-extract -Wuci -otest-WWuci-out.pgn --quiet test-ucW.pgn
	// -Wuci implicitly forces a very large (5000) output line width
	// (argsfile.c, OUTPUT_FORMAT_ARGUMENT case, "@@@ Warning: arbitrary
	// value."); pgnx's convert applies the same forced default for uci
	// (and no-op for others) unless -w was explicitly given. See
	// cmd/pgnx/convert.go's forcedLineWidth.
	{"test-WWuci", []string{"convert", "-f", "uci"}, "test-ucW.pgn", "test-WWuci-out.pgn"},
	// test-WW: pgn-extract -Wepd -otest-WWepd-out.pgn --quiet test-ucW.pgn
	{"test-WWepd", []string{"convert", "-f", "epd"}, "test-ucW.pgn", "test-WWepd-out.pgn"},

	// test-nocomments (-C): pgn-extract -C -otest-CC-out.pgn --quiet test-C.pgn
	{"test-CC", []string{"convert", "--no-comments"}, "test-C.pgn", "test-CC-out.pgn"},

	// test-output (-o/--output): pgn-extract -otest-o-out.pgn --quiet test-o.txt
	{"test-output", []string{"convert"}, "test-o.txt", "test-o-out.pgn"},

	// test-RR (-R roster.txt): pgn-extract -Rroster.txt --output test-RR-out.pgn --quiet test-R.pgn
	// roster.txt lists (alphabetically): Black, Date, Event, Result, Round,
	// Site, White; pgnx's --roster takes the same list as CSV instead of a
	// file path.
	{"test-RR", []string{"convert", "--roster", "Black,Date,Event,Result,Round,Site,White"}, "test-R.pgn", "test-RR-out.pgn"},

	// test-linelength (-w): pgn-extract -w60 -otest-w60-out.pgn --quiet test-w.pgn
	{"test-w60", []string{"convert", "-w", "60"}, "test-w.pgn", "test-w60-out.pgn"},
	// test-linelength (-w): pgn-extract -w75 -otest-w75-out.pgn --quiet test-w.pgn
	{"test-w75", []string{"convert", "-w", "75"}, "test-w.pgn", "test-w75-out.pgn"},
	// test-linelength (-w): pgn-extract -w1000 -otest-w1000-out.pgn --quiet test-w.pgn
	{"test-w1000", []string{"convert", "-w", "1000"}, "test-w.pgn", "test-w1000-out.pgn"},

	// test-nestedcomments: pgn-extract --nestedcomments -otest-nestedcomments-out.pgn --quiet nested-comment.pgn
	{"test-nestedcomments", []string{"convert", "--nested-comments"}, "nested-comment.pgn", "test-nestedcomments-out.pgn"},

	// test-long-line: pgn-extract -otest-long-line-out.pgn -ltest-long-line-log.txt test-long-line.pgn
	// (default flags; exercises a comment too long to fit within the
	// default line width)
	{"test-long-line", []string{"convert"}, "test-long-line.pgn", "test-long-line-out.pgn"},

	// test-promotion (plain conversion, not --underpromotion which is a
	// match/filter flag and out of scope): pgn-extract -otest-promotion-out.pgn --quiet test-promotion-in.pgn
	{"test-promotion", []string{"convert"}, "test-promotion-in.pgn", "test-promotion-out.pgn"},

	// test-f is not a row here: it combines two input files (-f files.txt),
	// which the single-input goldenCases table can't express. See
	// TestGoldenConformanceMultiFile below, which mirrors
	// TestGoldenConformanceAppend's pattern for the same reason.

	// test-s (-s, silent mode): pgn-extract -s -otest-s-out.pgn test-s.pgn.
	// pgnx has no distinct -s flag; -s (KEEP_SILENT_ARGUMENT, argsfile.h)
	// only suppresses the C oracle's per-run summary line, the same effect
	// as pgnx's --quiet, and the movetext output is unaffected either way
	// (verified byte-identical against the oracle run with --quiet instead
	// of -s).
	{"test-s", []string{"convert", "--quiet"}, "test-s.pgn", "test-s-out.pgn"},

	// test-l (-l log.txt, plain conversion + log file): pgn-extract
	// -llog.txt -otest-l-out.pgn fischer.pgn. Only test-l-out.pgn (not
	// log.txt) is in convert's comparison scope; -l's destination doesn't
	// affect movetext output, so this row omits --log entirely.
	{"test-l", []string{"convert"}, "fischer.pgn", "test-l-out.pgn"},

	// test-nomovenumbers (--nomovenumbers): pgn-extract -otest-nomovenumbers-out.pgn
	// --nomovenumbers --quiet test-nomovenumbers.pgn. Has its own dedicated
	// input/oracle pair distinct from test-VV, so it is its own row despite
	// exercising the same --no-move-numbers flag as no other row does.
	{"test-nomovenumbers", []string{"convert", "--no-move-numbers"}, "test-nomovenumbers.pgn", "test-nomovenumbers-out.pgn"},

	// test-noresults (--noresults): pgn-extract -otest-noresults-out.pgn
	// --noresults --quiet test-noresults.pgn. Has its own dedicated
	// input/oracle pair, so it is its own row despite exercising the same
	// --no-results flag as no other row does.
	{"test-noresults", []string{"convert", "--no-results"}, "test-noresults.pgn", "test-noresults-out.pgn"},

	// Phase 2+ (require filtering, selection, deduplication, ECO
	// classification, hashing, or other features not implemented by
	// `convert`; deliberately excluded from this harness):
	//   test-AA, test-b/-p (ply/move bounds), test-checkfile, test-duplicates,
	//   test-noduplicates, test-e/-EE (ECO), test-checkmate, test-n,
	//   test-suppressmatched, test-SS, test-t/-T,
	//   test-nounique, test-v, test-x, test-y, test-z, test-ZZ, test-hash
	//   (-# game-splitting, not implemented), test-evaluation, test-FF/
	//   test-fencomments (FEN-comment output, not implemented),
	//   test-markmatches, test-nochecks (new NoChecks output option, not
	//   yet implemented), test-notags (new NO_TAGS tag_output_format, not
	//   yet implemented), test-plylimit, test-stalemate, test-plycount,
	//   test-addhashcode (new PlyCount/TotalPlyCount/HashCode tag-adding
	//   options, not yet implemented), test-selectonly,
	//   test-fifty, test-repetition, test-fixresulttags (requires replay-
	//   derived Result correction, not yet implemented), test-fuzzydepth,
	//   test-setup, test-stopafter, test-skipmatching, test-splitvariants
	//   (structural game-splitting transform, not implemented),
	//   test-nobadresults, test-allownullmoves (null-move decoding, not
	//   implemented), test-matchplylimit,
	//   test-FENPattern, test-dropply (ply-trimming transform, not
	//   implemented), test-startply, test-linenumbers (line-number marker
	//   tag, not implemented),
	//   test-seventyfive, test-elodiff, test-reltags, test-ratedwins,
	//   test-odds, test-PP, test-r (report mode; the C Makefile itself
	//   doesn't byte-compare its output), test-h (help text, not
	//   conversion), test-LL (log-file-only comparison, no PGN output);
	//   sub-targets not promoted to their own row: -WsanBSLTDK (custom
	//   output piece letters within test-WW, no CLI surface for
	//   alternative piece letters yet), test-nagcomments (exercises NAG/
	//   comment interleaving order, e.g. "$1 { c } $2"; pgn.Move stores
	//   NAGs and Comments as separate slices with no shared ordering, so
	//   pgnx always renders all NAGs before all comments — a parser
	//   data-model change, not a minimal oracle-justified output fix);
	//   no-flags-1/no-flags-2 (fools-mate.pgn is vendored under infiles/,
	//   not outfiles/, and isn't a $(CMP) target in the C Makefile itself
	//   — it's illustrative, not an enforced oracle).
}

// TestGoldenConformance runs every row in goldenCases through the pgnx CLI
// and requires its output to byte-match the pgn-extract C oracle. No
// normalization or trimming is applied: any divergence, however small
// (whitespace, tag order, wrap points), is a failure.
func TestGoldenConformance(t *testing.T) {
	for _, c := range goldenCases {
		t.Run(c.name, func(t *testing.T) {
			outFile := filepath.Join(t.TempDir(), "out.pgn")
			args := append(append([]string{}, c.args...),
				"--quiet", "-o", outFile,
				filepath.Join("testdata", "golden", "infiles", c.input))
			root := NewRootCmd()
			var errb bytes.Buffer
			root.SetErr(&errb)
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("pgnx %v: %v\n%s", args, err, errb.String())
			}
			got, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatal(err)
			}
			want, err := os.ReadFile(filepath.Join("testdata", "golden", "outfiles", c.oracle))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from C oracle %s\n--- first divergence ---\n%s",
					c.oracle, firstDiff(got, want))
			}
		})
	}
}

// TestGoldenConformanceAppend mirrors the C Makefile's test-append target:
// pgn-extract --output test-a-out.pgn --quiet test-a.txt
// pgn-extract -atest-a-out.pgn --quiet test-a.txt
// (run twice: once with -a, once with --append). It needs two sequential
// invocations against the same output file, which the single-shot
// goldenCases table can't express.
func TestGoldenConformanceAppend(t *testing.T) {
	in := filepath.Join("testdata", "golden", "infiles", "test-a.txt")
	want, err := os.ReadFile(filepath.Join("testdata", "golden", "outfiles", "test-a-out.pgn"))
	if err != nil {
		t.Fatal(err)
	}

	for _, second := range []string{"-a", "--append"} {
		t.Run(second, func(t *testing.T) {
			outFile := filepath.Join(t.TempDir(), "out.pgn")

			root := NewRootCmd()
			root.SetArgs([]string{"convert", "--quiet", "-o", outFile, in})
			if err := root.Execute(); err != nil {
				t.Fatalf("first pgnx convert: %v", err)
			}

			root = NewRootCmd()
			var errb bytes.Buffer
			root.SetErr(&errb)
			root.SetArgs([]string{"convert", "--quiet", "--append", "-o", outFile, in})
			if err := root.Execute(); err != nil {
				t.Fatalf("second pgnx convert (%s): %v\n%s", second, err, errb.String())
			}

			got, err := os.ReadFile(outFile)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from C oracle test-a-out.pgn\n--- first divergence ---\n%s",
					firstDiff(got, want))
			}
		})
	}
}

// TestGoldenConformanceMultiFile mirrors the C Makefile's test-f target:
// pgn-extract --quiet -ftest-f-files.txt -otest-f-out.pgn
// where files.txt lists test-f1.pgn and test-f2.pgn. pgnx's convert has no
// -f-style "list of input files" flag; it accepts multiple positional file
// arguments directly, which is functionally identical to the C oracle
// reading files.txt's list (verified byte-identical against the oracle run
// with both files as positional args instead of -f). This needs two input
// files, which the single-input goldenCases table can't express.
func TestGoldenConformanceMultiFile(t *testing.T) {
	outFile := filepath.Join(t.TempDir(), "out.pgn")
	args := []string{"convert", "--quiet", "-o", outFile,
		filepath.Join("testdata", "golden", "infiles", "test-f1.pgn"),
		filepath.Join("testdata", "golden", "infiles", "test-f2.pgn")}

	root := NewRootCmd()
	var errb bytes.Buffer
	root.SetErr(&errb)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("pgnx %v: %v\n%s", args, err, errb.String())
	}
	got, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "golden", "outfiles", "test-f-out.pgn"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from C oracle test-f-out.pgn\n--- first divergence ---\n%s",
			firstDiff(got, want))
	}
}

func firstDiff(got, want []byte) string {
	gl := bytes.Split(got, []byte("\n"))
	wl := bytes.Split(want, []byte("\n"))
	for i := 0; i < len(gl) && i < len(wl); i++ {
		if !bytes.Equal(gl[i], wl[i]) {
			return "line " + strconv.Itoa(i+1) + ":\n got: " + string(gl[i]) + "\nwant: " + string(wl[i])
		}
	}
	return "length differs: got " + strconv.Itoa(len(gl)) + " lines, want " + strconv.Itoa(len(wl))
}
