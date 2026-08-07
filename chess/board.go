package chess

// CastlingRights tracks which castling moves are still available and, for
// each side, the file the castling rook started on. In standard chess the
// kingside rook starts on file 7 (h-file) and the queenside rook on file 0
// (a-file); Chess960 games may record other starting files.
type CastlingRights struct {
	WhiteKingside, WhiteQueenside bool
	BlackKingside, BlackQueenside bool

	WhiteKRookFile, WhiteQRookFile int
	BlackKRookFile, BlackQRookFile int
}

// Board represents a chess position.
type Board struct {
	SideToMove     Color
	Castling       CastlingRights
	HasEP          bool
	EPSquare       Square
	HalfmoveClock  int
	FullmoveNumber int

	squares [8][8]Piece // indexed [file][rank]
	kings   [3]Square   // indexed by Color
}

// startPositionFEN is the FEN for the standard chess starting position.
const startPositionFEN = "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"

// NewBoard returns a Board set up in the standard starting position.
func NewBoard() *Board {
	return mustFromFEN(startPositionFEN)
}

// mustFromFEN parses a FEN that is known to be valid (e.g. a compile-time
// constant); it panics on error, which indicates a programmer error rather
// than bad user input.
func mustFromFEN(fen string) *Board {
	b, err := FromFEN(fen)
	if err != nil {
		panic(err)
	}
	return b
}

// PieceAt returns the piece on the given square, or the zero Piece if empty.
func (b *Board) PieceAt(sq Square) Piece {
	return b.squares[sq.File][sq.Rank]
}

// setPiece places p on sq, updating the king-square cache when a king is
// placed or removed.
func (b *Board) setPiece(sq Square, p Piece) {
	old := b.squares[sq.File][sq.Rank]
	if old.Type == King && b.kings[old.Color] == sq {
		b.kings[old.Color] = Square{}
	}
	b.squares[sq.File][sq.Rank] = p
	if p.Type == King {
		b.kings[p.Color] = sq
	}
}

// KingSquare returns the cached square of the given color's king.
func (b *Board) KingSquare(c Color) Square {
	return b.kings[c]
}

// Copy returns an independent copy of the board.
func (b *Board) Copy() *Board {
	nb := *b
	return &nb
}
