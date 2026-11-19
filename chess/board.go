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

	// The placement, as bitboards (ADR 0005): the squares holding each
	// piece type and the squares holding each colour, so the pieces of
	// one kind and colour are byType[t] & byColor[c] and the occupied
	// squares byColor[White] | byColor[Black]. The mailbox holds the
	// same placement one packed piece code per square (pieceCode), for
	// PieceAt. setPiece is the only writer of all three.
	byType  [7]uint64 // indexed by PieceType; NoPiece unused
	byColor [3]uint64 // indexed by Color; NoColor unused
	mailbox [64]uint8 // indexed by sqIndex

	// placement is the PlacementKey of the pieces, maintained by setPiece
	// so that hashing a position never scans the board: the XOR of the
	// Polyglot piece-square term of every occupied square, 0 for an empty
	// board (so the zero Board is consistent). pgn-extract keeps its hash
	// values up to date in the same way as each move is applied (apply.c).
	placement uint64
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

// pieceCode packs p into the mailbox's byte: the colour in bits 3-4 and
// the type in bits 0-2, 0 for an empty square.
func pieceCode(p Piece) uint8 { return uint8(p.Color)<<3 | uint8(p.Type) }

// pieceOf unpacks a mailbox byte.
func pieceOf(code uint8) Piece { return Piece{Type: PieceType(code & 7), Color: Color(code >> 3)} }

// pieceSquareKeys is the Polyglot piece-square term of each piece code on
// each square, filled at init from polyglotRandom.
var pieceSquareKeys [24][64]uint64

func init() {
	for _, c := range []Color{White, Black} {
		for t := Pawn; t <= King; t++ {
			p := Piece{Type: t, Color: c}
			for s := 0; s < 64; s++ {
				pieceSquareKeys[pieceCode(p)][s] = polyglotRandom[64*polyglotKind(p)+s]
			}
		}
	}
}

// PieceAt returns the piece on the given square, or the zero Piece if empty.
func (b *Board) PieceAt(sq Square) Piece {
	return pieceOf(b.mailbox[sqIndex(sq)])
}

// setPiece places p on sq, updating the bitboards and the mailbox, and
// the placement key for the piece leaving and the piece arriving. It is
// the only writer of the placement, so the key is exact for every board
// built by FromFEN and every move applied since.
func (b *Board) setPiece(sq Square, p Piece) {
	s := sqIndex(sq)
	m := bit(s)
	if old := b.mailbox[s]; old != 0 {
		op := pieceOf(old)
		b.byType[op.Type] &^= m
		b.byColor[op.Color] &^= m
		b.placement ^= pieceSquareKeys[old][s]
	}
	code := pieceCode(p)
	b.mailbox[s] = code
	if code != 0 {
		b.byType[p.Type] |= m
		b.byColor[p.Color] |= m
		b.placement ^= pieceSquareKeys[code][s]
	}
}

// occupied is the mask of the occupied squares.
func (b *Board) occupied() uint64 { return b.byColor[White] | b.byColor[Black] }

// pieces is the mask of the squares holding c's pieces of type t.
func (b *Board) pieces(t PieceType, c Color) uint64 { return b.byType[t] & b.byColor[c] }

// kingIndex is the bit index of c's king, or 0 (a1) if c has no king.
func (b *Board) kingIndex(c Color) int {
	k := b.pieces(King, c)
	if k == 0 {
		return 0
	}
	return lowest(k)
}

// KingSquare returns the square of the given color's king, or the zero
// Square if that color has no king.
func (b *Board) KingSquare(c Color) Square {
	return sqOf(b.kingIndex(c))
}

// Copy returns an independent copy of the board.
func (b *Board) Copy() *Board {
	nb := *b
	return &nb
}
