package pgn

import (
	"bytes"
	"os"
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
