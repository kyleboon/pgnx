// Package replay decodes and regenerates Standard Algebraic Notation (SAN)
// move text against a chess.Board, by filtering the board's legal moves
// rather than fully re-implementing SAN's grammar.
package replay

import (
	"fmt"
	"strings"

	"github.com/kyleboon/pgnx/chess"
)

// pieceLetters maps the accepted (multilingual) leading piece letters to
// their PieceType, mirroring the C tool's decode.c:is_piece: K king; Q/D
// queen; R/T rook; N/S knight; B/L bishop.
var pieceLetters = map[byte]chess.PieceType{
	'K': chess.King,
	'Q': chess.Queen,
	'D': chess.Queen,
	'R': chess.Rook,
	'T': chess.Rook,
	'N': chess.Knight,
	'S': chess.Knight,
	'B': chess.Bishop,
	'L': chess.Bishop,
}

// pieceSANLetters maps a PieceType to its canonical (English) SAN letter,
// used when regenerating SAN text. Pawns have no letter.
var pieceSANLetters = map[chess.PieceType]string{
	chess.King:   "K",
	chess.Queen:  "Q",
	chess.Rook:   "R",
	chess.Knight: "N",
	chess.Bishop: "B",
}

// decodeSAN parses text (SAN or long-algebraic move notation) and returns
// the unique legal move on b that it identifies. It strips trailing
// annotation characters, handles castling, and otherwise filters
// b.LegalMoves() by piece type, destination, promotion piece, and any
// from-file/from-rank disambiguation hints present in text.
func decodeSAN(text string, b *chess.Board) (chess.Move, error) {
	orig := text
	s := strings.TrimSpace(text)

	// Step 1: strip trailing annotation characters and en-passant markers.
	s = strings.TrimRight(s, "!?+#")
	s = strings.TrimSuffix(s, "e.p.")
	s = strings.TrimSuffix(s, "ep")
	s = strings.TrimRight(s, "!?+#")

	if s == "" {
		return chess.Move{}, fmt.Errorf("illegal or unknown move %q", orig)
	}

	// Step 2: castling.
	if side := castleSide(s); side != chess.NoCastle {
		var match *chess.Move
		for _, m := range b.LegalMoves() {
			if m.Castle == side {
				if match != nil {
					return chess.Move{}, fmt.Errorf("ambiguous move %q", orig)
				}
				mCopy := m
				match = &mCopy
			}
		}
		if match == nil {
			return chess.Move{}, fmt.Errorf("illegal or unknown move %q", orig)
		}
		return *match, nil
	}

	// Step 3: core parse.
	pt, fromFile, fromRank, dest, promo, ok := parseCore(s)
	if !ok {
		return chess.Move{}, fmt.Errorf("illegal or unknown move %q", orig)
	}

	// Step 4: filter legal moves.
	var matches []chess.Move
	for _, m := range b.LegalMoves() {
		if m.Castle != chess.NoCastle {
			continue
		}
		p := b.PieceAt(m.From)
		if p.Type != pt {
			continue
		}
		if m.To != dest {
			continue
		}
		if promo != chess.NoPiece && m.Promotion != promo {
			continue
		}
		if promo == chess.NoPiece && m.Promotion != chess.NoPiece {
			continue
		}
		if fromFile != -1 && m.From.File != fromFile {
			continue
		}
		if fromRank != -1 && m.From.Rank != fromRank {
			continue
		}
		matches = append(matches, m)
	}

	switch len(matches) {
	case 0:
		return chess.Move{}, fmt.Errorf("illegal or unknown move %q", orig)
	case 1:
		return matches[0], nil
	default:
		return chess.Move{}, fmt.Errorf("ambiguous move %q", orig)
	}
}

// castleSide returns the CastleSide that s denotes, or chess.NoCastle if s
// is not castling notation.
func castleSide(s string) chess.CastleSide {
	switch strings.ToUpper(s) {
	case "O-O", "0-0":
		return chess.Kingside
	case "O-O-O", "0-0-0":
		return chess.Queenside
	}
	return chess.NoCastle
}

// parseCore parses the non-castling core of a SAN or long-algebraic move: an
// optional leading piece letter, optional from-file/from-rank disambiguation
// (or, for long algebraic notation, a full from-square), an optional capture
// separator, the destination square, and an optional promotion suffix. It
// returns ok=false if s cannot be parsed into this shape.
func parseCore(s string) (pt chess.PieceType, fromFile, fromRank int, dest chess.Square, promo chess.PieceType, ok bool) {
	fromFile, fromRank = -1, -1
	pt = chess.Pawn

	i := 0
	// Optional leading piece letter.
	if i < len(s) {
		if p, isPiece := pieceLetters[s[i]]; isPiece {
			pt = p
			i++
		}
	}

	// Remaining characters up to an optional promotion suffix are square /
	// disambiguation / capture-separator characters.
	rest := s[i:]

	// Split off promotion suffix: "=Q", "Q" (bare), or "/Q" at the end.
	// Only recognized when what remains before it still parses as
	// square(+disambiguation) text.
	promo = chess.NoPiece
	if len(rest) >= 2 {
		last := rest[len(rest)-1]
		if promoPT, isPiece := pieceLetters[last]; isPiece && promoPT != chess.King {
			// Try stripping a promotion suffix of 1, 2 ("=Q"), or 2 ("/Q") chars.
			switch {
			case rest[len(rest)-2] == '=' || rest[len(rest)-2] == '/':
				promo = promoPT
				rest = rest[:len(rest)-2]
			default:
				// Bare letter suffix, e.g. "a8Q": only treat as promotion if
				// the piece type was not already consumed as a leading
				// letter (pawn moves only) and there's more content before it
				// that looks like a destination square.
				if pt == chess.Pawn && len(rest) >= 3 {
					promo = promoPT
					rest = rest[:len(rest)-1]
				}
			}
		}
	}

	// rest should now end with a destination square (2 chars: file+rank),
	// optionally preceded by disambiguation (file and/or rank) and/or a
	// capture separator, all optionally preceded by a leading from-square
	// for long algebraic notation.
	if len(rest) < 2 {
		return 0, 0, 0, chess.Square{}, 0, false
	}

	destStr := rest[len(rest)-2:]
	destSq, err := chess.ParseSquare(destStr)
	if err != nil {
		return 0, 0, 0, chess.Square{}, 0, false
	}
	dest = destSq

	pre := rest[:len(rest)-2]

	// Long algebraic separator: e2-e4.
	pre = strings.TrimSuffix(pre, "-")

	// Capture separator: x, X, or :.
	pre = strings.TrimSuffix(pre, "x")
	pre = strings.TrimSuffix(pre, "X")
	pre = strings.TrimSuffix(pre, ":")

	if pre == "" {
		return pt, fromFile, fromRank, dest, promo, true
	}

	// A leading full square (long algebraic from-square), e.g. "e2" in
	// "e2e4" or "e2-e4": both a valid file letter and rank digit.
	if len(pre) == 2 && pre[0] >= 'a' && pre[0] <= 'h' && pre[1] >= '1' && pre[1] <= '8' {
		from, err := chess.ParseSquare(pre)
		if err == nil {
			return pt, from.File, from.Rank, dest, promo, true
		}
	}

	// Otherwise pre is disambiguation: an optional file letter and/or an
	// optional rank digit, in either order, at most one of each.
	for _, c := range []byte(pre) {
		switch {
		case c >= 'a' && c <= 'h' && fromFile == -1:
			fromFile = int(c - 'a')
		case c >= '1' && c <= '8' && fromRank == -1:
			fromRank = int(c - '1')
		default:
			return 0, 0, 0, chess.Square{}, 0, false
		}
	}

	return pt, fromFile, fromRank, dest, promo, true
}

// sanFor returns the normalized SAN text for move m on board b, computed
// before m is applied to b. It does not include a trailing check/mate
// suffix (the caller appends that after applying the move).
func sanFor(m chess.Move, b *chess.Board) string {
	if m.Castle == chess.Kingside {
		return "O-O"
	}
	if m.Castle == chess.Queenside {
		return "O-O-O"
	}

	mover := b.PieceAt(m.From)
	capture := isCapture(m, b)

	var sb strings.Builder
	if mover.Type == chess.Pawn {
		if capture {
			sb.WriteByte(byte('a' + m.From.File))
			sb.WriteByte('x')
		}
		sb.WriteString(m.To.String())
		if m.Promotion != chess.NoPiece {
			sb.WriteByte('=')
			sb.WriteString(pieceSANLetters[m.Promotion])
		}
		return sb.String()
	}

	sb.WriteString(pieceSANLetters[mover.Type])
	sb.WriteString(disambiguation(m, b, mover))
	if capture {
		sb.WriteByte('x')
	}
	sb.WriteString(m.To.String())
	return sb.String()
}

// isCapture reports whether m, played on b, captures an enemy piece
// (including en passant, whose captured pawn sits beside the destination
// square rather than on it).
func isCapture(m chess.Move, b *chess.Board) bool {
	if b.PieceAt(m.To).Type != chess.NoPiece {
		return true
	}
	mover := b.PieceAt(m.From)
	if mover.Type == chess.Pawn && b.HasEP && m.To == b.EPSquare {
		return true
	}
	return false
}

// disambiguation computes the minimal SAN disambiguation prefix for m among
// all legal moves by same-type, same-color pieces that reach m.To: empty if
// m.From is the only such origin; the from-file if that alone distinguishes
// it; else the from-rank if that alone distinguishes it; else both.
func disambiguation(m chess.Move, b *chess.Board, mover chess.Piece) string {
	var rivals []chess.Square
	for _, other := range b.LegalMoves() {
		if other.Castle != chess.NoCastle {
			continue
		}
		if other.To != m.To {
			continue
		}
		if other.From == m.From {
			continue
		}
		p := b.PieceAt(other.From)
		if p.Type != mover.Type || p.Color != mover.Color {
			continue
		}
		rivals = append(rivals, other.From)
	}
	if len(rivals) == 0 {
		return ""
	}

	fileUnique := true
	rankUnique := true
	for _, r := range rivals {
		if r.File == m.From.File {
			fileUnique = false
		}
		if r.Rank == m.From.Rank {
			rankUnique = false
		}
	}
	if fileUnique {
		return string(rune('a' + m.From.File))
	}
	if rankUnique {
		return string(rune('1' + m.From.Rank))
	}
	return m.From.String()
}
