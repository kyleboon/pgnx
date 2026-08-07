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

// Zobrist computes the Polyglot-compatible Zobrist hash of the position.
func (b *Board) Zobrist() uint64 {
	var h uint64

	for file := 0; file < 8; file++ {
		for rank := 0; rank < 8; rank++ {
			p := b.squares[file][rank]
			if p.Type == NoPiece {
				continue
			}
			kind := polyglotKind(p)
			h ^= polyglotRandom[64*kind+8*rank+file]
		}
	}

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

// epCapturable reports whether a pawn of the side to move stands adjacent
// (file ±1) to the EP-captured pawn, i.e. on the rank from which it would
// capture onto b.EPSquare. This is the Polyglot rule: the EP file is only
// hashed in if an en passant capture is actually available.
func epCapturable(b *Board) bool {
	ep := b.EPSquare
	// The capturing pawn sits on the same rank as the pawn that just moved
	// two squares, which is adjacent to the EP square's rank: one rank
	// toward the side to move relative to the EP square.
	var capturingRank int
	if b.SideToMove == White {
		capturingRank = ep.Rank - 1
	} else {
		capturingRank = ep.Rank + 1
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
