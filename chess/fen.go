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

// parseCastling parses the FEN castling-availability field. It accepts the
// standard "KQkq" notation (which implies rook start files 7 and 0) and
// Shredder-FEN style file letters (A-H/a-h), which record the actual rook
// starting file for Chess960 positions. "-" means no castling rights.
// The board's piece placement must already be parsed so the king squares
// are available for classifying Shredder letters.
func parseCastling(field string, b *Board) (CastlingRights, error) {
	if field == "-" {
		return CastlingRights{}, nil
	}
	// KQkq is the standard notation; ABCDEFGHabcdefgh (Shredder-FEN) records
	// actual rook files for Chess960. A field cannot mix the two styles.
	if strings.ContainsAny(field, "ABCDEFGHabcdefgh") {
		return parseShredderCastling(field, b)
	}

	var c CastlingRights
	for _, ch := range field {
		switch ch {
		case 'K':
			c.WhiteKRookFile, c.WhiteKingside = outermostRook(b, White, true)
		case 'Q':
			c.WhiteQRookFile, c.WhiteQueenside = outermostRook(b, White, false)
		case 'k':
			c.BlackKRookFile, c.BlackKingside = outermostRook(b, Black, true)
		case 'q':
			c.BlackQRookFile, c.BlackQueenside = outermostRook(b, Black, false)
		default:
			return CastlingRights{}, fmt.Errorf("fen: bad castling char %q", ch)
		}
	}
	return c, nil
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

// parseShredderCastling parses Shredder-FEN style castling rights, where
// each letter names the file of a castling-rook. Uppercase letters are
// White's rooks, lowercase are Black's. A rook file greater than the
// king's file is a kingside right; a smaller file is queenside. A letter
// equal to the king's file is invalid, as is naming a right for a side
// whose king is not on the board.
func parseShredderCastling(field string, b *Board) (CastlingRights, error) {
	var c CastlingRights
	set := func(color Color, file int, kingside, queenside *bool, kFile, qFile *int) error {
		kingSq := b.KingSquare(color)
		if b.PieceAt(kingSq) != (Piece{Type: King, Color: color}) {
			return fmt.Errorf("fen: castling rights for side with no king")
		}
		switch {
		case file > kingSq.File:
			if *kingside {
				return fmt.Errorf("fen: duplicate kingside castling file")
			}
			*kingside = true
			*kFile = file
		case file < kingSq.File:
			if *queenside {
				return fmt.Errorf("fen: duplicate queenside castling file")
			}
			*queenside = true
			*qFile = file
		default:
			return fmt.Errorf("fen: castling rook file %c equals king file", 'a'+file)
		}
		return nil
	}
	for _, ch := range field {
		var err error
		switch {
		case ch >= 'A' && ch <= 'H':
			err = set(White, int(ch-'A'),
				&c.WhiteKingside, &c.WhiteQueenside, &c.WhiteKRookFile, &c.WhiteQRookFile)
		case ch >= 'a' && ch <= 'h':
			err = set(Black, int(ch-'a'),
				&c.BlackKingside, &c.BlackQueenside, &c.BlackKRookFile, &c.BlackQRookFile)
		default:
			err = fmt.Errorf("fen: bad castling char %q", ch)
		}
		if err != nil {
			return CastlingRights{}, err
		}
	}
	return c, nil
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

// castlingFEN renders castling rights. When the rook files match the
// standard chess start (7 for kingside, 0 for queenside) it emits the usual
// KQkq notation; otherwise (Chess960) it emits Shredder-FEN file letters.
func castlingFEN(c CastlingRights) string {
	if !c.WhiteKingside && !c.WhiteQueenside && !c.BlackKingside && !c.BlackQueenside {
		return "-"
	}

	standard := (!c.WhiteKingside || c.WhiteKRookFile == 7) &&
		(!c.WhiteQueenside || c.WhiteQRookFile == 0) &&
		(!c.BlackKingside || c.BlackKRookFile == 7) &&
		(!c.BlackQueenside || c.BlackQRookFile == 0)

	var sb strings.Builder
	if standard {
		if c.WhiteKingside {
			sb.WriteByte('K')
		}
		if c.WhiteQueenside {
			sb.WriteByte('Q')
		}
		if c.BlackKingside {
			sb.WriteByte('k')
		}
		if c.BlackQueenside {
			sb.WriteByte('q')
		}
		return sb.String()
	}

	if c.WhiteKingside {
		sb.WriteByte(byte('A' + c.WhiteKRookFile))
	}
	if c.WhiteQueenside {
		sb.WriteByte(byte('A' + c.WhiteQRookFile))
	}
	if c.BlackKingside {
		sb.WriteByte(byte('a' + c.BlackKRookFile))
	}
	if c.BlackQueenside {
		sb.WriteByte(byte('a' + c.BlackQRookFile))
	}
	return sb.String()
}
