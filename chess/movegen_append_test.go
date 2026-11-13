package chess

import (
	"reflect"
	"testing"
)

// generatorFENs are positions with castling, en passant, pins, checks and
// promotions in play, so the generator is exercised on every move kind;
// the first two are also the benchmark positions.
var generatorFENs = []struct{ name, fen string }{
	{"start", startPositionFEN},
	{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1"},
	{"ep-pins", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1"},
	{"promotions", "r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1"},
	{"in-check", "rnb1kbnr/pppp1ppp/8/4p3/6Pq/5P2/PPPPP2P/RNBQKBNR w KQkq - 1 3"},
}

func TestAppendLegalMovesAgreesWithLegalMoves(t *testing.T) {
	for _, c := range generatorFENs {
		b := mustFromFEN(c.fen)
		want := b.LegalMoves()
		if got := b.AppendLegalMoves(nil); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: AppendLegalMoves(nil) = %v, want %v", c.name, got, want)
		}
	}
}

func TestAppendLegalMovesAppendsAfterExistingContents(t *testing.T) {
	b := NewBoard()
	prefix := []Move{{From: MustSquare("a1"), To: MustSquare("h8")}}
	got := b.AppendLegalMoves(prefix)
	if len(got) != 1+20 || got[0] != prefix[0] {
		t.Fatalf("AppendLegalMoves(prefix) = %d moves starting %v, want 21 starting %v", len(got), got[0], prefix[0])
	}
	if !reflect.DeepEqual(got[1:], b.LegalMoves()) {
		t.Errorf("moves after the prefix differ from LegalMoves")
	}
}

func TestAppendLegalMovesReusesCapacity(t *testing.T) {
	for _, c := range generatorFENs {
		b := mustFromFEN(c.fen)
		buf := make([]Move, 0, 256)
		got := b.AppendLegalMoves(buf[:0])
		// Same backing array: the result's first element is buf's.
		if len(got) > 0 && &got[0] != &buf[:1][0] {
			t.Errorf("%s: AppendLegalMoves reallocated although the buffer had capacity", c.name)
		}
		allocs := testing.AllocsPerRun(100, func() {
			buf = b.AppendLegalMoves(buf[:0])
		})
		if allocs != 0 {
			t.Errorf("%s: AppendLegalMoves into a buffer with capacity: %v allocs/op, want 0", c.name, allocs)
		}
	}
}
