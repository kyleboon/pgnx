package chess

import "fmt"

type Color uint8

const (
	NoColor Color = iota
	White
	Black
)

func (c Color) Opponent() Color {
	switch c {
	case White:
		return Black
	case Black:
		return White
	}
	return NoColor
}

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

func ParseSquare(name string) (Square, error) {
	if len(name) != 2 || name[0] < 'a' || name[0] > 'h' || name[1] < '1' || name[1] > '8' {
		return Square{}, fmt.Errorf("invalid square %q", name)
	}
	return Square{File: int(name[0] - 'a'), Rank: int(name[1] - '1')}, nil
}

func MustSquare(name string) Square {
	sq, err := ParseSquare(name)
	if err != nil {
		panic(err)
	}
	return sq
}
