package chess

// CastleSide identifies which side of the board a castling move castles
// toward, or NoCastle for a non-castling move.
type CastleSide uint8

const (
	NoCastle CastleSide = iota
	Kingside
	Queenside
)

// Move represents a single chess move. For castling moves, From and To are
// the king's start and end squares (e.g. e1->g1 for White kingside); in
// Chess960, To is still the king's normal destination file (g-file for
// kingside, c-file for queenside), even though the rook may start on that
// square.
type Move struct {
	From, To  Square
	Promotion PieceType
	Castle    CastleSide
	// Null marks a null move ("--" in pgn-extract's PGN): the side to
	// move passes. From, To, Promotion and Castle are then meaningless.
	Null bool
}

// NullMove is the null move: the turn passes without a piece moving
// (pgn-extract's NULL_MOVE, written "--"). Board.Apply flips the side to
// move and advances the fullmove number after Black, and changes nothing
// else, as the C tool's apply_move does for it.
var NullMove = Move{Null: true}

// IsNull reports whether m is the null move.
func (m Move) IsNull() bool { return m.Null }

// promotionLetters maps a promotion piece type to its lowercase UCI letter.
var promotionLetters = map[PieceType]byte{
	Knight: 'n',
	Bishop: 'b',
	Rook:   'r',
	Queen:  'q',
}

// UCI returns the move in UCI long algebraic notation, e.g. "e2e4",
// "e7e8q", or "e1g1" for castling; "0000" for the null move.
func (m Move) UCI() string {
	if m.IsNull() {
		return "0000"
	}
	s := m.From.String() + m.To.String()
	if m.Promotion != NoPiece {
		if letter, ok := promotionLetters[m.Promotion]; ok {
			s += string(letter)
		}
	}
	return s
}
