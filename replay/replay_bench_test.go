package replay

import (
	"os"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
)

// BenchmarkGame replays the fixed bundle in testdata/bench.pgn once per
// iteration: real games with castling, checks and promotions, plus games
// with variations (nested, with castling and checks inside them) and
// checkmates. Besides the per-bundle ns/op and allocs/op it reports the
// per-game and per-ply figures ADR 0003's throughput work tracks, counting
// every ply replayed, variations included. Every game and every variation
// in the bundle must replay in full, so a nil variation fails the
// benchmark rather than silently shrinking the ply count.
func BenchmarkGame(b *testing.B) {
	games := loadBenchGames(b)
	plies := 0
	for i, g := range games {
		r, err := Game(g, Options{})
		if err != nil {
			b.Fatalf("game %d: %v", i+1, err)
		}
		n, ok := countPlies(r)
		if !ok {
			b.Fatalf("game %d: a variation failed to replay", i+1)
		}
		plies += n
	}
	replayAll := func() {
		for _, g := range games {
			if _, err := Game(g, Options{}); err != nil {
				b.Fatal(err)
			}
		}
	}

	// Allocations per bundle pass, measured the way -benchmem measures
	// allocs/op (a runtime.MemStats delta), so the per-game and per-ply
	// figures agree with it.
	allocs := testing.AllocsPerRun(1, replayAll)

	b.ReportAllocs()
	for b.Loop() {
		replayAll()
	}
	nsPerPass := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	b.ReportMetric(float64(len(games)), "games/op")
	b.ReportMetric(float64(plies), "plies/op")
	b.ReportMetric(nsPerPass/float64(len(games)), "ns/game")
	b.ReportMetric(allocs/float64(len(games)), "allocs/game")
	b.ReportMetric(nsPerPass/float64(plies), "ns/ply")
	b.ReportMetric(allocs/float64(plies), "allocs/ply")
}

func loadBenchGames(b *testing.B) []*pgn.Game {
	b.Helper()
	f, err := os.Open("testdata/bench.pgn")
	if err != nil {
		b.Fatal(err)
	}
	defer f.Close()
	var games []*pgn.Game
	s := pgn.NewScanner(f, pgn.ParserOptions{})
	for s.Next() {
		games = append(games, s.Game())
	}
	if err := s.Err(); err != nil {
		b.Fatal(err)
	}
	if len(games) == 0 {
		b.Fatal("no games in testdata/bench.pgn")
	}
	return games
}

// countPlies counts the plies r replayed, main line and variations, and
// reports false if any variation failed to replay (recorded as nil).
func countPlies(r *Result) (int, bool) {
	n := len(r.Moves)
	for _, m := range r.Moves {
		for _, v := range m.Variations {
			if v == nil {
				return 0, false
			}
			vn, ok := countPlies(v)
			if !ok {
				return 0, false
			}
			n += vn
		}
	}
	return n, true
}
