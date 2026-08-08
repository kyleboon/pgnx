// Package chess implements a chess board, move generation, and move
// application, used by the pgn and replay packages to validate and replay
// the movetext of parsed PGN games.
package chess

import "fmt"

// Color identifies a side: White, Black, or the zero-value NoColor.
type Color uint8

const (
	NoColor Color = iota
	White
	Black
)

// Opponent returns the other side, or NoColor if c is NoColor.
func (c Color) Opponent() Color {
	switch c {
	case White:
		return Black
	case Black:
		return White
	}
	return NoColor
}

// PieceType identifies a kind of chess piece, independent of color, or the
// zero-value NoPiece for an empty square.
type PieceType uint8

const (
	NoPiece PieceType = iota
	Pawn
	Knight
	Bishop
	Rook
	Queen
	King
)

// Piece is a piece kind paired with the color that owns it.
type Piece struct {
	Type  PieceType
	Color Color
}

// Square identifies a board square. File 0..7 = a..h, Rank 0..7 = 1..8.
type Square struct{ File, Rank int }

func (s Square) Valid() bool { return s.File >= 0 && s.File < 8 && s.Rank >= 0 && s.Rank < 8 }

func (s Square) String() string {
	return string(rune('a'+s.File)) + string(rune('1'+s.Rank))
}

// ParseSquare parses algebraic square notation (e.g. "e4") into a Square.
// It returns an error if name is not exactly two characters in the range
// a-h, 1-8.
func ParseSquare(name string) (Square, error) {
	if len(name) != 2 || name[0] < 'a' || name[0] > 'h' || name[1] < '1' || name[1] > '8' {
		return Square{}, fmt.Errorf("invalid square %q", name)
	}
	return Square{File: int(name[0] - 'a'), Rank: int(name[1] - '1')}, nil
}

// MustSquare is like ParseSquare but panics if name is invalid. It is
// intended for tests and package-level tables with fixed, known-good
// square names.
func MustSquare(name string) Square {
	sq, err := ParseSquare(name)
	if err != nil {
		panic(err)
	}
	return sq
}
