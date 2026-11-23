package pgn

import (
	"bytes"
	"os"
	"runtime"
	"testing"
)

// BenchmarkScanner parses the replay package's bench bundle
// (../replay/testdata/bench.pgn: real games with comments, NAGs and
// nested variations) once per iteration, scanner only, no replay. Besides
// ns/op and allocs/op per bundle pass it reports the per-game figures
// ADR 0003's throughput work tracks for the lexer and parser.
func BenchmarkScanner(b *testing.B) {
	src, err := os.ReadFile("../replay/testdata/bench.pgn")
	if err != nil {
		b.Fatal(err)
	}
	scanAll := func() int {
		n := 0
		s := NewScanner(bytes.NewReader(src), ParserOptions{})
		for s.Next() {
			n++
		}
		if err := s.Err(); err != nil {
			b.Fatal(err)
		}
		return n
	}
	games := scanAll()
	if games == 0 {
		b.Fatal("no games in the bench bundle")
	}
	allocs := testing.AllocsPerRun(1, func() { scanAll() })

	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	for b.Loop() {
		scanAll()
	}
	nsPerPass := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	b.ReportMetric(float64(games), "games/op")
	b.ReportMetric(nsPerPass/float64(games), "ns/game")
	b.ReportMetric(allocs/float64(games), "allocs/game")
}

// BenchmarkScannerCorpus is the scanner-only pass over a whole PGN file
// that ADR 0003's lexer rows report: set PGNX_SCAN_CORPUS to the file
// (the TWIC slice, say; an absolute path, since go test runs in the
// package directory) and each iteration streams it once through
// pgn.Scanner with no replay, reporting MB/s, games/op and allocs/game.
// It is skipped when the variable is unset, so `mise run bench` does not
// need the corpus. A few iterations are plenty:
//
//	PGNX_SCAN_CORPUS=$PWD/slice.pgn go test ./pgn -run '^$' -bench ScannerCorpus -benchtime 3x
func BenchmarkScannerCorpus(b *testing.B) {
	path := os.Getenv("PGNX_SCAN_CORPUS")
	if path == "" {
		b.Skip("PGNX_SCAN_CORPUS not set")
	}
	info, err := os.Stat(path)
	if err != nil {
		b.Fatal(err)
	}
	scanAll := func() int {
		f, err := os.Open(path)
		if err != nil {
			b.Fatal(err)
		}
		defer f.Close()
		n := 0
		s := NewScanner(f, ParserOptions{})
		for s.Next() {
			n++
		}
		if err := s.Err(); err != nil {
			b.Fatal(err)
		}
		return n
	}

	b.ReportAllocs()
	b.SetBytes(info.Size())
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	games := 0
	for b.Loop() {
		games = scanAll()
	}
	runtime.ReadMemStats(&after)
	if games == 0 {
		b.Fatal("no games in the corpus")
	}
	allocs := float64(after.Mallocs-before.Mallocs) / float64(b.N)
	b.ReportMetric(float64(games), "games/op")
	b.ReportMetric(allocs/float64(games), "allocs/game")
}
