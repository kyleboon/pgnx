package chess

// Polyglot offsets within the 781-entry random table.
const (
	polyglotCastleOffset = 768 // 4 entries: WK, WQ, BK, BQ
	polyglotEPOffset     = 772 // 8 entries: one per file
	polyglotTurnOffset   = 780 // 1 entry: set when White to move
)

// polyglotKind returns the Polyglot piece index for p, per the ordering:
// black pawn=0, white pawn=1, black knight=2, white knight=3, black bishop=4,
// white bishop=5, black rook=6, white rook=7, black queen=8, white queen=9,
// black king=10, white king=11.
func polyglotKind(p Piece) int {
	var base int
	switch p.Type {
	case Pawn:
		base = 0
	case Knight:
		base = 2
	case Bishop:
		base = 4
	case Rook:
		base = 6
	case Queen:
		base = 8
	case King:
		base = 10
	}
	if p.Color == White {
		base++
	}
	return base
}

// PlacementKey returns a hash of the piece placement alone: the XOR of
// the Polyglot piece-square terms over every occupied square, with no
// castling, en passant or side-to-move contribution. The board maintains
// it as pieces are placed and removed, so the call is O(1). Two boards
// with the same pieces on the same squares share a key regardless of
// whose move it is, which rights remain, or the clock fields.
//
// It is pgnx's stand-in for pgn-extract's weak_hash_value, which drives
// positional matching (-x, -Tf, -H), ECO classification and duplicate
// detection in the C tool; only equality of the key is observable.
func (b *Board) PlacementKey() uint64 {
	return b.placement
}

// Zobrist computes the Polyglot-compatible Zobrist hash of the position.
func (b *Board) Zobrist() uint64 {
	h := b.PlacementKey()

	if b.Castling.WhiteKingside {
		h ^= polyglotRandom[polyglotCastleOffset+0]
	}
	if b.Castling.WhiteQueenside {
		h ^= polyglotRandom[polyglotCastleOffset+1]
	}
	if b.Castling.BlackKingside {
		h ^= polyglotRandom[polyglotCastleOffset+2]
	}
	if b.Castling.BlackQueenside {
		h ^= polyglotRandom[polyglotCastleOffset+3]
	}

	if b.HasEP && epCapturable(b) {
		h ^= polyglotRandom[polyglotEPOffset+b.EPSquare.File]
	}

	if b.SideToMove == White {
		h ^= polyglotRandom[polyglotTurnOffset]
	}

	return h
}

// epCapturable reports whether a pawn of the side to move stands beside
// the pawn that would be captured en passant, i.e. on the rank from which
// it would capture onto b.EPSquare: White's fifth rank, Black's fourth.
// This is the Polyglot rule, as pgn-extract's zobrist.c applies it: the
// EP file is only hashed in if an en passant capture is actually
// available. The rank is fixed per side rather than derived from the EP
// square so that the square a null move leaves behind ("1. e4 --" keeps
// e3 with White to move) does not count White's own second-rank pawns,
// matching the C tool.
func epCapturable(b *Board) bool {
	ep := b.EPSquare
	capturingRank := 4 // fifth rank: a white pawn capturing a black one
	if b.SideToMove == Black {
		capturingRank = 3 // fourth rank
	}

	for _, df := range [...]int{-1, 1} {
		f := ep.File + df
		if f < 0 || f > 7 {
			continue
		}
		p := b.squares[f][capturingRank]
		if p.Type == Pawn && p.Color == b.SideToMove {
			return true
		}
	}
	return false
}
