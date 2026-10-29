package main

import (
	"bytes"
	"errors"
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
// Rows cover convert-equivalent behavior (pure move/tag output formatting)
// and, from Phase 2, the filter command's selection criteria. See the
// "Phase 3+" comment at the bottom of this table for targets deliberately
// excluded (positional matching, ECO classification, deduplication, and
// output transforms not yet implemented).
var goldenCases = []struct {
	name   string
	args   []string // pgnx args; input/output paths are appended by the runner
	input  string   // file under testdata/golden/infiles
	oracle string   // file under testdata/golden/outfiles
	// wantProblems accepts exit status 2 (errProblems): the row's input
	// deliberately contains games pgnx rejects (and reports) while the C
	// oracle silently drops them. Any other error still fails the row.
	wantProblems bool
}{
	// test-7: pgn-extract -7 -otest-7-out.pgn --quiet test-7.pgn
	{"test-7", []string{"convert", "--seven-tag-roster"}, "test-7.pgn", "test-7-out.pgn", false},
	// test-NN: pgn-extract -N -otest-NN-out.pgn --quiet test-N.pgn
	{"test-NN", []string{"convert", "--no-nags"}, "test-N.pgn", "test-NN-out.pgn", false},
	// test-VV: pgn-extract -V -otest-VV-out.pgn --quiet test-V.pgn
	{"test-VV", []string{"convert", "--no-variations"}, "test-V.pgn", "test-VV-out.pgn", false},
	// test-WW: pgn-extract -Whalg -otest-WWhalg-out.pgn --quiet test-ucW.pgn
	{"test-WWhalg", []string{"convert", "-f", "halg"}, "test-ucW.pgn", "test-WWhalg-out.pgn", false},
	// test-WW: pgn-extract -Wlalg -otest-WWlalg-out.pgn --quiet test-ucW.pgn
	{"test-WWlalg", []string{"convert", "-f", "lalg"}, "test-ucW.pgn", "test-WWlalg-out.pgn", false},
	// test-WW: pgn-extract -Welalg -otest-WWelalg-out.pgn --quiet test-ucW.pgn
	{"test-WWelalg", []string{"convert", "-f", "elalg"}, "test-ucW.pgn", "test-WWelalg-out.pgn", false},
	// test-WW: pgn-extract -Wxlalg -otest-WWxlalg-out.pgn --quiet test-ucW.pgn
	{"test-WWxlalg", []string{"convert", "-f", "xlalg"}, "test-ucW.pgn", "test-WWxlalg-out.pgn", false},
	// test-WW: pgn-extract -Wuci -otest-WWuci-out.pgn --quiet test-ucW.pgn
	// -Wuci implicitly forces a very large (5000) output line width
	// (argsfile.c, OUTPUT_FORMAT_ARGUMENT case, "@@@ Warning: arbitrary
	// value."); pgnx's convert applies the same forced default for uci
	// (and no-op for others) unless -w was explicitly given. See
	// cmd/pgnx/convert.go's forcedLineWidth.
	{"test-WWuci", []string{"convert", "-f", "uci"}, "test-ucW.pgn", "test-WWuci-out.pgn", false},
	// test-WW: pgn-extract -Wepd -otest-WWepd-out.pgn --quiet test-ucW.pgn
	{"test-WWepd", []string{"convert", "-f", "epd"}, "test-ucW.pgn", "test-WWepd-out.pgn", false},

	// test-nocomments (-C): pgn-extract -C -otest-CC-out.pgn --quiet test-C.pgn
	{"test-CC", []string{"convert", "--no-comments"}, "test-C.pgn", "test-CC-out.pgn", false},

	// test-output (-o/--output): pgn-extract -otest-o-out.pgn --quiet test-o.txt
	{"test-output", []string{"convert"}, "test-o.txt", "test-o-out.pgn", false},

	// test-RR (-R roster.txt): pgn-extract -Rroster.txt --output test-RR-out.pgn --quiet test-R.pgn
	// roster.txt lists (alphabetically): Black, Date, Event, Result, Round,
	// Site, White; pgnx's --roster takes the same list as CSV instead of a
	// file path.
	{"test-RR", []string{"convert", "--roster", "Black,Date,Event,Result,Round,Site,White"}, "test-R.pgn", "test-RR-out.pgn", false},

	// test-linelength (-w): pgn-extract -w60 -otest-w60-out.pgn --quiet test-w.pgn
	{"test-w60", []string{"convert", "-w", "60"}, "test-w.pgn", "test-w60-out.pgn", false},
	// test-linelength (-w): pgn-extract -w75 -otest-w75-out.pgn --quiet test-w.pgn
	{"test-w75", []string{"convert", "-w", "75"}, "test-w.pgn", "test-w75-out.pgn", false},
	// test-linelength (-w): pgn-extract -w1000 -otest-w1000-out.pgn --quiet test-w.pgn
	{"test-w1000", []string{"convert", "-w", "1000"}, "test-w.pgn", "test-w1000-out.pgn", false},

	// test-nestedcomments: pgn-extract --nestedcomments -otest-nestedcomments-out.pgn --quiet nested-comment.pgn
	{"test-nestedcomments", []string{"convert", "--nested-comments"}, "nested-comment.pgn", "test-nestedcomments-out.pgn", false},

	// test-long-line: pgn-extract -otest-long-line-out.pgn -ltest-long-line-log.txt test-long-line.pgn
	// (default flags; exercises a comment too long to fit within the
	// default line width)
	{"test-long-line", []string{"convert"}, "test-long-line.pgn", "test-long-line-out.pgn", false},

	// test-promotion (plain conversion, not --underpromotion which is a
	// match/filter flag and out of scope): pgn-extract -otest-promotion-out.pgn --quiet test-promotion-in.pgn
	{"test-promotion", []string{"convert"}, "test-promotion-in.pgn", "test-promotion-out.pgn", false},

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
	{"test-s", []string{"convert", "--quiet"}, "test-s.pgn", "test-s-out.pgn", false},

	// test-l (-l log.txt, plain conversion + log file): pgn-extract
	// -llog.txt -otest-l-out.pgn fischer.pgn. Only test-l-out.pgn (not
	// log.txt) is in convert's comparison scope; -l's destination doesn't
	// affect movetext output, so this row omits --log entirely.
	{"test-l", []string{"convert"}, "fischer.pgn", "test-l-out.pgn", false},

	// test-nomovenumbers (--nomovenumbers): pgn-extract -otest-nomovenumbers-out.pgn
	// --nomovenumbers --quiet test-nomovenumbers.pgn. Has its own dedicated
	// input/oracle pair distinct from test-VV, so it is its own row despite
	// exercising the same --no-move-numbers flag as no other row does.
	{"test-nomovenumbers", []string{"convert", "--no-move-numbers"}, "test-nomovenumbers.pgn", "test-nomovenumbers-out.pgn", false},

	// test-noresults (--noresults): pgn-extract -otest-noresults-out.pgn
	// --noresults --quiet test-noresults.pgn. Has its own dedicated
	// input/oracle pair, so it is its own row despite exercising the same
	// --no-results flag as no other row does.
	{"test-noresults", []string{"convert", "--no-results"}, "test-noresults.pgn", "test-noresults-out.pgn", false},

	// ---- Phase 2: pgnx filter ----

	// test-bl45: pgn-extract -bl45 -otest-bl45-out.pgn --quiet fischer.pgn
	{"test-bl45", []string{"filter", "--min-moves", "45"}, "fischer.pgn", "test-bl45-out.pgn", false},
	// test-bu45: pgn-extract -bu45 -otest-bu45-out.pgn --quiet fischer.pgn
	{"test-bu45", []string{"filter", "--max-moves", "45"}, "fischer.pgn", "test-bu45-out.pgn", false},
	// test-bu30: pgn-extract -b30 -otest-b30-out.pgn --quiet fischer.pgn
	{"test-b30", []string{"filter", "--moves", "30"}, "fischer.pgn", "test-b30-out.pgn", false},
	// test-pl90: pgn-extract -pl90 -otest-pl90-out.pgn --quiet fischer.pgn
	{"test-pl90", []string{"filter", "--min-ply", "90"}, "fischer.pgn", "test-pl90-out.pgn", false},
	// test-pu90: pgn-extract -pu90 -otest-pu90-out.pgn --quiet fischer.pgn
	{"test-pu90", []string{"filter", "--max-ply", "90"}, "fischer.pgn", "test-pu90-out.pgn", false},
	// test-pu60: pgn-extract -p60 -otest-p60-out.pgn --quiet fischer.pgn
	{"test-p60", []string{"filter", "--ply", "60"}, "fischer.pgn", "test-p60-out.pgn", false},

	// test-TT: pgn-extract -Td1970 -otest-TTd-out.pgn --quiet fischer.pgn
	{"test-TTd", []string{"filter", "--date", "1970"}, "fischer.pgn", "test-TTd-out.pgn", false},
	// test-TT: pgn-extract -Td1960 -Td1962 -otest-TTdd-out.pgn --quiet fischer.pgn
	{"test-TTdd", []string{"filter", "--date", "1960", "--date", "1962"}, "fischer.pgn", "test-TTdd-out.pgn", false},
	// test-TT: pgn-extract -TbPetrosian -otest-TTb-out.pgn --quiet fischer.pgn
	{"test-TTb", []string{"filter", "--black", "Petrosian"}, "fischer.pgn", "test-TTb-out.pgn", false},
	// test-TT: pgn-extract -TpPetrosian -otest-TTp-out.pgn --quiet fischer.pgn
	{"test-TTp", []string{"filter", "--player", "Petrosian"}, "fischer.pgn", "test-TTp-out.pgn", false},
	// test-TT: pgn-extract -Tr0-1 -otest-TTr-out.pgn --quiet fischer.pgn
	{"test-TTr", []string{"filter", "--result", "0-1"}, "fischer.pgn", "test-TTr-out.pgn", false},
	// test-TT: pgn-extract -TwFischer -otest-TTw-out.pgn --quiet fischer.pgn
	{"test-TTw", []string{"filter", "--white", "Fischer"}, "fischer.pgn", "test-TTw-out.pgn", false},
	// test-TT: pgn-extract -TaBarnes -otest-TTa-out.pgn --quiet test-Ta.pgn
	// (-Ta has no dedicated shorthand flag; --tag with the -t grammar is
	// the same criterion.)
	{"test-TTa", []string{"filter", "--tag", `Annotator "Barnes"`}, "test-Ta.pgn", "test-TTa-out.pgn", false},

	// test-t: pgn-extract -ttaglist.txt -otest-t-out.pgn --quiet test-t.pgn
	{"test-t", []string{"filter", "--tag-file", filepath.Join("testdata", "golden", "infiles", "taglist.txt")}, "test-t.pgn", "test-t-out.pgn", false},
	// test-reltags: pgn-extract -treltags.txt -otest-reltags-out.pgn --quiet bots.pgn
	{"test-reltags", []string{"filter", "--tag-file", filepath.Join("testdata", "golden", "infiles", "reltags.txt")}, "bots.pgn", "test-reltags-out.pgn", false},
	// test-elodiff: pgn-extract -t test-elodiff.txt -otest-elodiff-out.pgn --quiet test-seventyfive.pgn
	{"test-elodiff", []string{"filter", "--tag-file", filepath.Join("testdata", "golden", "infiles", "test-elodiff.txt")}, "test-seventyfive.pgn", "test-elodiff-out.pgn", false},
	// test-SS: pgn-extract -S -TpPetrosian -otest-SS-out.pgn --quiet test-ucS.pgn
	{"test-SS", []string{"filter", "--soundex", "--player", "Petrosian"}, "test-ucS.pgn", "test-SS-out.pgn", false},

	// test-ratedwins: pgn-extract --quiet --higherratedwinner infiles/bots.pgn -o test-higherratedwins-out.pgn
	{"test-higherratedwins", []string{"filter", "--higher-rated-winner"}, "bots.pgn", "test-higherratedwins-out.pgn", false},
	// test-ratedwins: pgn-extract --quiet --lowerratedwinner infiles/bots.pgn -o test-lowerratedwins-out.pgn
	{"test-lowerratedwins", []string{"filter", "--lower-rated-winner"}, "bots.pgn", "test-lowerratedwins-out.pgn", false},

	// test-setup: pgn-extract --nosetuptags -otest-nosetup-out.pgn --quiet test-setup-in.pgn
	{"test-nosetup", []string{"filter", "--no-setup-tags"}, "test-setup-in.pgn", "test-nosetup-out.pgn", false},
	// test-setup: pgn-extract --onlysetuptags -otest-onlysetup-out.pgn --quiet test-setup-in.pgn
	{"test-onlysetup", []string{"filter", "--only-setup-tags"}, "test-setup-in.pgn", "test-onlysetup-out.pgn", false},

	// test-checkmate: pgn-extract --checkmate -otest-checkmate-out.pgn --quiet test-checkmate.pgn
	{"test-checkmate", []string{"filter", "--checkmate"}, "test-checkmate.pgn", "test-checkmate-out.pgn", false},
	// test-stalemate: pgn-extract --stalemate -otest-stalemate-out.pgn --quiet test-stalemate.pgn
	{"test-stalemate", []string{"filter", "--stalemate"}, "test-stalemate.pgn", "test-stalemate-out.pgn", false},
	// test-fifty: pgn-extract --fifty --markmatches FIFTY -otest-fifty-out.pgn --quiet test-fifty.pgn
	{"test-fifty", []string{"filter", "--fifty", "--mark-matches", "FIFTY"}, "test-fifty.pgn", "test-fifty-out.pgn", false},
	// test-seventyfive: pgn-extract --seventyfive --markmatches SEVENTYFIVE -otest-seventyfive-out.pgn --quiet test-seventyfive.pgn
	{"test-seventyfive", []string{"filter", "--seventy-five", "--mark-matches", "SEVENTYFIVE"}, "test-seventyfive.pgn", "test-seventyfive-out.pgn", false},
	// test-repetition: pgn-extract --repetition --markmatches REPEAT -otest-repetition-out.pgn --quiet test-repetition.pgn
	{"test-repetition", []string{"filter", "--repetition", "--mark-matches", "REPEAT"}, "test-repetition.pgn", "test-repetition-out.pgn", false},
	// test-promotion (2nd command): pgn-extract --underpromotion -otest-underpromotion-out.pgn --quiet test-promotion-in.pgn
	{"test-underpromotion", []string{"filter", "--underpromotion"}, "test-promotion-in.pgn", "test-underpromotion-out.pgn", false},

	// test-z: pgn-extract -zzmatch.txt -otest-z-out.pgn --markmatches MATCH --quiet fischer.pgn
	{"test-z-file", []string{"filter", "--material-file", filepath.Join("testdata", "golden", "infiles", "zmatch.txt"), "--mark-matches", "MATCH"}, "fischer.pgn", "test-z-out.pgn", false},
	// test-z: pgn-extract --materialz "q*r*p*b2n2< q=r=p=b2<n2" -otest-z-out.pgn --markmatches MATCH --quiet fischer.pgn
	{"test-z-flag", []string{"filter", "--material", "q*r*p*b2n2< q=r=p=b2<n2", "--mark-matches", "MATCH"}, "fischer.pgn", "test-z-out.pgn", false},
	// test-y: pgn-extract -yymatch.txt --markmatches MATCH -otest-y-out.pgn --quiet fischer.pgn
	{"test-y-file", []string{"filter", "--white-material-file", filepath.Join("testdata", "golden", "infiles", "ymatch.txt"), "--mark-matches", "MATCH"}, "fischer.pgn", "test-y-out.pgn", false},
	// test-y: pgn-extract --materialy "q*r*p*b2n2< q=r=p=b2<n2" --markmatches MATCH -otest-y-out.pgn --quiet fischer.pgn
	{"test-y-flag", []string{"filter", "--white-material", "q*r*p*b2n2< q=r=p=b2<n2", "--mark-matches", "MATCH"}, "fischer.pgn", "test-y-out.pgn", false},

	// test-selectonly: pgn-extract -TpPetrosian --selectonly 2,3 -otest-selectonly-out.pgn --quiet test-selectonly.pgn
	{"test-selectonly", []string{"filter", "--player", "Petrosian", "--select-only", "2,3"}, "test-selectonly.pgn", "test-selectonly-out.pgn", false},
	// test-skipmatching: pgn-extract -TpKeres --skipmatching 1,2 -otest-skipmatching-out.pgn --quiet fischer.pgn
	{"test-skipmatching", []string{"filter", "--player", "Keres", "--skip-matching", "1,2"}, "fischer.pgn", "test-skipmatching-out.pgn", false},
	// test-stopafter: pgn-extract -TpPetrosian --stopafter 1 -otest-stopafter-out.pgn --quiet test-stopafter.pgn
	{"test-stopafter", []string{"filter", "--player", "Petrosian", "--stop-after", "1"}, "test-stopafter.pgn", "test-stopafter-out.pgn", false},

	// test-nobadresults: pgn-extract --nobadresults -otest-nobadresults-out.pgn -ltest-nobadresults-log.txt --quiet test-nobadresults.pgn
	// (the rejected games make pgnx exit 2, hence wantProblems)
	{"test-nobadresults", []string{"filter", "--no-bad-results"}, "test-nobadresults.pgn", "test-nobadresults-out.pgn", true},
	// test-fixresulttags: pgn-extract --fixresulttags -otest-fixresulttags-out.pgn -lfix-results-log.txt --quiet test-fixresulttags-in.pgn
	{"test-fixresulttags", []string{"filter", "--fix-result-tags"}, "test-fixresulttags-in.pgn", "test-fixresulttags-out.pgn", false},

	// test-n and test-suppressmatched need two outputs; see
	// TestGoldenConformanceUnmatched and TestGoldenConformanceSuppressMatched.

	// Phase 3+ (deliberately excluded from this harness):
	//   Positional matching (Phase 3): test-tFEN (FEN lines in -t files),
	//   test-Te (needs -e ECO classification), test-x, test-v,
	//   test-markmatches (uses -x), test-matchplylimit, test-startply,
	//   test-FENPattern, test-PP.
	//   ECO classification (Phase 3): test-e/-EE.
	//   Duplicate detection (Phase 4): test-AA, test-checkfile,
	//   test-duplicates, test-noduplicates, test-nounique, test-ZZ,
	//   test-fuzzydepth.
	//   Null moves: test-allownullmoves, test-odds (null-move decoding, not
	//   implemented).
	//   Output transforms not yet implemented ("Phase 2b"): test-hash (-#
	//   game-splitting), test-evaluation, test-FF/test-fencomments
	//   (FEN-comment output), test-nochecks (NoChecks output option),
	//   test-notags (NO_TAGS tag_output_format), test-plylimit,
	//   test-plycount, test-addhashcode (PlyCount/TotalPlyCount/HashCode
	//   tag-adding options), test-splitvariants (structural game-splitting
	//   transform), test-dropply (ply-trimming transform), test-linenumbers
	//   (line-number marker tag).
	//   Not byte-compared by the C Makefile either: test-r (report mode),
	//   test-h (help text), test-LL (log-file-only comparison);
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
			err := root.Execute()
			if c.wantProblems && errors.Is(err, errProblems) {
				err = nil
			}
			if err != nil {
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

// TestGoldenConformanceUnmatched mirrors the C Makefile's test-n target:
// pgn-extract -TpFischer -otest-n-matched.pgn -ntest-n-unmatched.pgn --quiet petrosian.pgn
// Two output files are compared, which the goldenCases table can't
// express.
func TestGoldenConformanceUnmatched(t *testing.T) {
	dir := t.TempDir()
	matched := filepath.Join(dir, "matched.pgn")
	unmatched := filepath.Join(dir, "unmatched.pgn")
	args := []string{"filter", "--quiet", "--player", "Fischer", "-o", matched, "--unmatched", unmatched,
		filepath.Join("testdata", "golden", "infiles", "petrosian.pgn")}

	root := NewRootCmd()
	var errb bytes.Buffer
	root.SetErr(&errb)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("pgnx %v: %v\n%s", args, err, errb.String())
	}
	for _, c := range []struct{ got, oracle string }{
		{matched, "test-n-matched.pgn"},
		{unmatched, "test-n-unmatched.pgn"},
	} {
		got, err := os.ReadFile(c.got)
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
	}
}

// TestGoldenConformanceSuppressMatched mirrors the C Makefile's
// test-suppressmatched target:
// pgn-extract -TpPetrosian --suppressmatched -nnon-petrosian.pgn --quiet fischer.pgn
// The matched games (which would go to stdout) must be suppressed
// entirely, and only the unmatched file is compared.
func TestGoldenConformanceSuppressMatched(t *testing.T) {
	unmatched := filepath.Join(t.TempDir(), "non-petrosian.pgn")
	args := []string{"filter", "--quiet", "--player", "Petrosian", "--suppress-matched", "--unmatched", unmatched,
		filepath.Join("testdata", "golden", "infiles", "fischer.pgn")}

	root := NewRootCmd()
	var out, errb bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errb)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("pgnx %v: %v\n%s", args, err, errb.String())
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty with --suppress-matched, got %d bytes", out.Len())
	}
	got, err := os.ReadFile(unmatched)
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", "golden", "outfiles", "non-petrosian.pgn"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output differs from C oracle non-petrosian.pgn\n--- first divergence ---\n%s",
			firstDiff(got, want))
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
