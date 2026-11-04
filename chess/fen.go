package chess

import (
	"fmt"
	"strconv"
	"strings"
)

// FromFEN parses a FEN (or 4-field EPD-style position) string into a Board.
func FromFEN(fen string) (*Board, error) {
	fields := strings.Fields(fen)
	if len(fields) != 4 && len(fields) != 6 {
		return nil, fmt.Errorf("fen: want 4 or 6 fields, got %d", len(fields))
	}

	b := &Board{}

	ranks := strings.Split(fields[0], "/")
	if len(ranks) != 8 {
		return nil, fmt.Errorf("fen: want 8 ranks, got %d", len(ranks))
	}
	for r := 0; r < 8; r++ {
		file := 0
		for _, ch := range ranks[7-r] { // FEN lists rank 8 first
			if ch >= '1' && ch <= '8' {
				file += int(ch - '0')
				if file > 8 {
					return nil, fmt.Errorf("fen: rank %d overflows", r+1)
				}
			} else {
				p, ok := pieceFromFENChar(ch)
				if !ok || file > 7 {
					return nil, fmt.Errorf("fen: bad char %q", ch)
				}
				b.setPiece(Square{file, r}, p)
				file++
			}
		}
		if file != 8 {
			return nil, fmt.Errorf("fen: rank %d has %d files", r+1, file)
		}
	}

	switch fields[1] {
	case "w":
		b.SideToMove = White
	case "b":
		b.SideToMove = Black
	default:
		return nil, fmt.Errorf("fen: bad side to move %q", fields[1])
	}

	castling, err := parseCastling(fields[2], b)
	if err != nil {
		return nil, err
	}
	b.Castling = castling

	if fields[3] == "-" {
		b.HasEP = false
	} else {
		sq, err := ParseSquare(fields[3])
		if err != nil {
			return nil, fmt.Errorf("fen: bad en passant square %q: %w", fields[3], err)
		}
		b.HasEP = true
		b.EPSquare = sq
	}

	if len(fields) == 6 {
		hm, err := strconv.Atoi(fields[4])
		if err != nil {
			return nil, fmt.Errorf("fen: bad halfmove clock %q: %w", fields[4], err)
		}
		b.HalfmoveClock = hm

		fm, err := strconv.Atoi(fields[5])
		if err != nil {
			return nil, fmt.Errorf("fen: bad fullmove number %q: %w", fields[5], err)
		}
		b.FullmoveNumber = fm
	} else {
		b.HalfmoveClock = 0
		b.FullmoveNumber = 1
	}

	return b, nil
}

func pieceFromFENChar(ch rune) (Piece, bool) {
	var color Color
	if ch >= 'a' && ch <= 'z' {
		color = Black
	} else if ch >= 'A' && ch <= 'Z' {
		color = White
	} else {
		return Piece{}, false
	}

	var pt PieceType
	switch ch {
	case 'p', 'P':
		pt = Pawn
	case 'n', 'N':
		pt = Knight
	case 'b', 'B':
		pt = Bishop
	case 'r', 'R':
		pt = Rook
	case 'q', 'Q':
		pt = Queen
	case 'k', 'K':
		pt = King
	default:
		return Piece{}, false
	}
	return Piece{Type: pt, Color: color}, true
}

func fenCharFromPiece(p Piece) rune {
	var ch rune
	switch p.Type {
	case Pawn:
		ch = 'p'
	case Knight:
		ch = 'n'
	case Bishop:
		ch = 'b'
	case Rook:
		ch = 'r'
	case Queen:
		ch = 'q'
	case King:
		ch = 'k'
	}
	if p.Color == White {
		ch = ch - 'a' + 'A'
	}
	return ch
}

// parseCastling parses the FEN castling-availability field: "-" for no
// rights, or letters naming each right. K/Q/k/q (standard FEN, and X-FEN
// for Chess960) grant the right for the outermost rook on that wing of
// the king's back rank, as pgn-extract's find_rook_starting_position
// does; A-H/a-h (Shredder-FEN) name the castling rook's file, a file
// greater than the king's being a kingside right and a smaller one
// queenside. The two styles may be mixed, as the C tool mixes them when
// it writes a field (see castlingFEN). A letter equal to the king's file,
// a right given twice, or a right for a side whose king is not on the
// board is an error. The board's piece placement must already be parsed
// so the kings and rooks can be found.
func parseCastling(field string, b *Board) (CastlingRights, error) {
	if field == "-" {
		return CastlingRights{}, nil
	}
	var c CastlingRights
	for _, ch := range field {
		var err error
		switch {
		case ch == 'K' || ch == 'Q' || (ch >= 'A' && ch <= 'H'):
			err = grantCastling(b, White, ch, &c.WhiteKingside, &c.WhiteQueenside, &c.WhiteKRookFile, &c.WhiteQRookFile)
		case ch == 'k' || ch == 'q' || (ch >= 'a' && ch <= 'h'):
			err = grantCastling(b, Black, ch-'a'+'A', &c.BlackKingside, &c.BlackQueenside, &c.BlackKRookFile, &c.BlackQRookFile)
		default:
			err = fmt.Errorf("fen: bad castling char %q", ch)
		}
		if err != nil {
			return CastlingRights{}, err
		}
	}
	return c, nil
}

// grantCastling records the right one uppercase castling letter (K, Q or
// a file letter A-H) gives color, in that color's fields of a
// CastlingRights.
func grantCastling(b *Board, color Color, letter rune, kingside, queenside *bool, kFile, qFile *int) error {
	kingSq := b.KingSquare(color)
	if b.PieceAt(kingSq) != (Piece{Type: King, Color: color}) {
		return fmt.Errorf("fen: castling rights for side with no king")
	}
	var file int
	var isKingside bool
	switch letter {
	case 'K', 'Q':
		isKingside = letter == 'K'
		var ok bool
		if file, ok = outermostRook(b, color, isKingside); !ok {
			// No rook on that wing: the right is dropped, as the C tool
			// drops it.
			return nil
		}
	default:
		file = int(letter - 'A')
		if file == kingSq.File {
			return fmt.Errorf("fen: castling rook file %c equals king file", 'a'+file)
		}
		isKingside = file > kingSq.File
	}
	if isKingside {
		if *kingside {
			return fmt.Errorf("fen: duplicate kingside castling right")
		}
		*kingside, *kFile = true, file
		return nil
	}
	if *queenside {
		return fmt.Errorf("fen: duplicate queenside castling right")
	}
	*queenside, *qFile = true, file
	return nil
}

// outermostRook finds the rook of the given colour nearest the h-file
// (kingside) or a-file (queenside) on that colour's back rank, as
// pgn-extract's find_rook_starting_position does for KQkq. ok is false when
// the back rank holds no such rook, in which case no right is granted.
func outermostRook(b *Board, color Color, kingside bool) (file int, ok bool) {
	rank := 0
	if color == Black {
		rank = 7
	}
	want := Piece{Type: Rook, Color: color}
	if kingside {
		for f := 7; f >= 0; f-- {
			if b.PieceAt(Square{File: f, Rank: rank}) == want {
				return f, true
			}
		}
		return 0, false
	}
	for f := 0; f < 8; f++ {
		if b.PieceAt(Square{File: f, Rank: rank}) == want {
			return f, true
		}
	}
	return 0, false
}

// FEN returns the FEN string for the board.
func (b *Board) FEN() string {
	var sb strings.Builder

	for r := 7; r >= 0; r-- {
		empty := 0
		for f := 0; f < 8; f++ {
			p := b.squares[f][r]
			if p.Type == NoPiece {
				empty++
				continue
			}
			if empty > 0 {
				sb.WriteString(strconv.Itoa(empty))
				empty = 0
			}
			sb.WriteRune(fenCharFromPiece(p))
		}
		if empty > 0 {
			sb.WriteString(strconv.Itoa(empty))
		}
		if r > 0 {
			sb.WriteByte('/')
		}
	}

	sb.WriteByte(' ')
	if b.SideToMove == Black {
		sb.WriteByte('b')
	} else {
		sb.WriteByte('w')
	}

	sb.WriteByte(' ')
	sb.WriteString(castlingFEN(b.Castling))

	sb.WriteByte(' ')
	if b.HasEP {
		sb.WriteString(b.EPSquare.String())
	} else {
		sb.WriteByte('-')
	}

	sb.WriteByte(' ')
	sb.WriteString(strconv.Itoa(b.HalfmoveClock))
	sb.WriteByte(' ')
	sb.WriteString(strconv.Itoa(b.FullmoveNumber))

	return sb.String()
}

// castlingFEN spells the castling field as pgn-extract's
// build_basic_EPD_string (apply.c) does. A side with both rights is
// written K and Q (k and q) whatever files its rooks start on: X-FEN,
// where the letter names the outermost rook on that wing, which is what
// parseCastling resolves it back to. (The C tool writes both file
// letters instead when its kingside file is not the greater, which its
// own Shredder-letter parsing can produce; a CastlingRights always has
// the kingside file greater, so pgnx has no such case.) A side with one right is written
// K or Q only when that rook starts on its standard file (h or a);
// otherwise the right is the rook's file letter, as in Shredder-FEN, so
// a field may mix the two styles ("Kd"), which parseCastling accepts.
// (A right for the inner of two rooks on one wing is therefore not
// round-tripped, as in the C tool.)
func castlingFEN(c CastlingRights) string {
	var sb strings.Builder
	side := func(kingside, queenside bool, kFile, qFile int, k, q byte, a byte) {
		switch {
		case kingside && queenside:
			sb.WriteByte(k)
			sb.WriteByte(q)
		case kingside && kFile == 7:
			sb.WriteByte(k)
		case kingside:
			sb.WriteByte(a + byte(kFile))
		case queenside && qFile == 0:
			sb.WriteByte(q)
		case queenside:
			sb.WriteByte(a + byte(qFile))
		}
	}
	side(c.WhiteKingside, c.WhiteQueenside, c.WhiteKRookFile, c.WhiteQRookFile, 'K', 'Q', 'A')
	side(c.BlackKingside, c.BlackQueenside, c.BlackKRookFile, c.BlackQRookFile, 'k', 'q', 'a')
	if sb.Len() == 0 {
		return "-"
	}
	return sb.String()
}
