// Package replay decodes and regenerates Standard Algebraic Notation (SAN)
// move text against a chess.Board, by filtering the legal moves the text
// can denote (those of the piece type it names to the square it names,
// or every legal move when it names no piece) rather than fully
// re-implementing SAN's grammar.
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

// promotionLetters maps the accepted trailing promotion letters to their
// PieceType, mirroring decode.c's is_piece (called on the promotion suffix)
// plus its explicit "allow a trailing 'b' as a Bishop promotion" case.
// is_piece is case-insensitive only for K/k, Q/q, R/r, N/n; its other
// Dutch/German letters (D, T, S) are uppercase-only there, since lowercase
// 'd' is a file letter and must not be captured as a promotion suffix.
// Lowercase 'b' is separately special-cased in decode.c as a Bishop
// promotion (but not lowercase 'l'). This covers long-algebraic
// d7d8r/d7d8n/d7d8b/d7d8q and bare SAN d8q.
var promotionLetters = map[byte]chess.PieceType{
	'K': chess.King, 'k': chess.King,
	'Q': chess.Queen, 'q': chess.Queen, 'D': chess.Queen,
	'R': chess.Rook, 'r': chess.Rook, 'T': chess.Rook,
	'N': chess.Knight, 'n': chess.Knight, 'S': chess.Knight,
	'B': chess.Bishop, 'b': chess.Bishop, 'L': chess.Bishop,
}

// decodeSAN parses text (SAN or long-algebraic move notation) and returns
// the unique legal move of b that it identifies, reading the moves from
// rp's buffer. It strips trailing annotation characters, handles castling,
// and otherwise filters the candidates by piece type, destination,
// promotion piece, and any from-file/from-rank disambiguation hints
// present in text. A move naming its piece (or a pawn move) and a
// destination needs only the candidates of that piece type to that
// square; castling text asks the board for the one move it names; bare
// long-algebraic notation, whose from-square alone names the piece,
// reads every legal move. It allocates nothing on the way to a move.
func decodeSAN(text string, b *chess.Board, rp *replayer) (chess.Move, error) {
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
		// A position has at most one castling move per side, so the
		// board's probe finds it without the other legal moves.
		match, ok := b.CastleMove(side)
		if !ok {
			return chess.Move{}, fmt.Errorf("illegal or unknown move %q", orig)
		}
		return match, nil
	}

	// Step 3: core parse.
	pt, fromFile, fromRank, dest, promo, anyPiece, ok := parseCore(s)
	if !ok {
		return chess.Move{}, fmt.Errorf("illegal or unknown move %q", orig)
	}

	// Step 4: filter the candidates, keeping the one match or failing
	// on the second.
	var candidates []chess.Move
	if anyPiece {
		candidates = rp.legalMoves(b)
	} else {
		candidates = rp.candidates(b, pt, dest)
	}
	var match chess.Move
	matches := 0
	for _, m := range candidates {
		if m.Castle != chess.NoCastle && !anyPiece {
			// Castling is normally matched only via O-O/O-O-O text (Step
			// 2); King moves can't otherwise land on a castle move's
			// King.To. The exception is bare long-algebraic notation with
			// no leading piece letter (e.g. "e1g1", "e8c8"), which the C
			// oracle also accepts as castling since the king's own
			// from/to squares happen to match the rook-side castle move.
			continue
		}
		if !anyPiece {
			p := b.PieceAt(m.From)
			if p.Type != pt {
				continue
			}
		}
		if m.To != dest {
			continue
		}
		if promo != chess.NoPiece && m.Promotion != promo {
			continue
		}
		if promo == chess.NoPiece && m.Promotion != chess.NoPiece && m.Promotion != chess.Queen {
			// Implicit promotion: a pawn move written without a promotion
			// piece that reaches the last rank is a queen promotion, as in
			// map.c determine_move_details ("Since the move string doesn't
			// tell us, we have to assume a queen"). Keeping only the queen
			// move leaves exactly one candidate.
			continue
		}
		if fromFile != -1 && m.From.File != fromFile {
			continue
		}
		if fromRank != -1 && m.From.Rank != fromRank {
			continue
		}
		if matches++; matches > 1 {
			return chess.Move{}, fmt.Errorf("ambiguous move %q", orig)
		}
		match = m
	}
	if matches == 0 {
		return chess.Move{}, fmt.Errorf("illegal or unknown move %q", orig)
	}
	return match, nil
}

// castleSide returns the CastleSide that s denotes, or chess.NoCastle if s
// is not castling notation: letter O in either case, or digit 0.
func castleSide(s string) chess.CastleSide {
	switch {
	case strings.EqualFold(s, "O-O"), s == "0-0":
		return chess.Kingside
	case strings.EqualFold(s, "O-O-O"), s == "0-0-0":
		return chess.Queenside
	}
	return chess.NoCastle
}

// parseCore parses the non-castling core of a SAN or long-algebraic move: an
// optional leading piece letter, optional from-file/from-rank disambiguation
// (or, for long algebraic notation, a full from-square), an optional capture
// separator, the destination square, and an optional promotion suffix. It
// returns ok=false if s cannot be parsed into this shape.
//
// anyPiece reports that no leading piece letter was present and the from
// hint turned out to be a full square (both file and rank), e.g. "c1b2":
// bare long-algebraic notation for a non-pawn move, which the C oracle
// (decode.c) accepts by locating the piece from the from-square rather
// than requiring a leading letter. In that case pt is left as chess.Pawn
// only as a zero value; callers must not filter on piece type and should
// instead rely on the from-square (which alone disambiguates a legal
// move) — see decodeSAN's Step 4.
func parseCore(s string) (pt chess.PieceType, fromFile, fromRank int, dest chess.Square, promo chess.PieceType, anyPiece, ok bool) {
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
	hadLeadingLetter := i > 0

	// Remaining characters up to an optional promotion suffix are square /
	// disambiguation / capture-separator characters.
	rest := s[i:]

	// Split off promotion suffix: "=Q", "Q" (bare), or "/Q" at the end.
	// Only recognized when what remains before it still parses as
	// square(+disambiguation) text.
	promo = chess.NoPiece
	if len(rest) >= 2 {
		last := rest[len(rest)-1]
		if promoPT, isPiece := promotionLetters[last]; isPiece && promoPT != chess.King {
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
		return 0, 0, 0, chess.Square{}, 0, false, false
	}

	destStr := rest[len(rest)-2:]
	destSq, err := chess.ParseSquare(destStr)
	if err != nil {
		return 0, 0, 0, chess.Square{}, 0, false, false
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
		return pt, fromFile, fromRank, dest, promo, false, true
	}

	// A leading full square (long algebraic from-square), e.g. "e2" in
	// "e2e4" or "e2-e4": both a valid file letter and rank digit.
	if len(pre) == 2 && pre[0] >= 'a' && pre[0] <= 'h' && pre[1] >= '1' && pre[1] <= '8' {
		from, err := chess.ParseSquare(pre)
		if err == nil {
			// No leading piece letter and a full from-square: bare
			// long-algebraic notation for any piece (e.g. "c1b2" for a
			// bishop). The from-square alone disambiguates, so the piece
			// type must not be forced to pawn (decode.c locates the piece
			// from the from-square rather than requiring a letter).
			anyPiece = !hadLeadingLetter
			return pt, from.File, from.Rank, dest, promo, anyPiece, true
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
			return 0, 0, 0, chess.Square{}, 0, false, false
		}
	}

	return pt, fromFile, fromRank, dest, promo, false, true
}

// appendSAN appends the normalized SAN text for move m on board b to dst
// and returns the extended slice, computed before m is applied to b. The
// other moves of the same piece type to m.To, which disambiguation must
// tell m from, are read from rp's buffer. The text has no trailing
// check/mate suffix (the caller appends that after applying the move).
func appendSAN(dst []byte, m chess.Move, b *chess.Board, rp *replayer) []byte {
	if m.Castle == chess.Kingside {
		return append(dst, "O-O"...)
	}
	if m.Castle == chess.Queenside {
		return append(dst, "O-O-O"...)
	}

	mover := b.PieceAt(m.From)
	capture := isCapture(m, b)

	if mover.Type == chess.Pawn {
		if capture {
			dst = append(dst, byte('a'+m.From.File), 'x')
		}
		dst = appendSquare(dst, m.To)
		if m.Promotion != chess.NoPiece {
			dst = append(dst, '=')
			dst = append(dst, pieceSANLetters[m.Promotion]...)
		}
		return dst
	}

	dst = append(dst, pieceSANLetters[mover.Type]...)
	dst = appendDisambiguation(dst, m, b, mover, rp.candidates(b, mover.Type, m.To))
	if capture {
		dst = append(dst, 'x')
	}
	return appendSquare(dst, m.To)
}

// appendSquare appends sq in algebraic notation ("e4") to dst.
func appendSquare(dst []byte, sq chess.Square) []byte {
	return append(dst, byte('a'+sq.File), byte('1'+sq.Rank))
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

// appendDisambiguation appends the minimal SAN disambiguation prefix for
// m among candidates, legal moves of b that include every move of a piece
// like mover to m.To: nothing if m.From is the only origin of a
// same-type, same-color piece reaching m.To; the from-file if that alone
// distinguishes it; else the from-rank if that alone does; else both.
func appendDisambiguation(dst []byte, m chess.Move, b *chess.Board, mover chess.Piece, candidates []chess.Move) []byte {
	rivals := 0
	fileUnique, rankUnique := true, true
	for _, other := range candidates {
		if other.Castle != chess.NoCastle || other.To != m.To || other.From == m.From {
			continue
		}
		if p := b.PieceAt(other.From); p.Type != mover.Type || p.Color != mover.Color {
			continue
		}
		rivals++
		if other.From.File == m.From.File {
			fileUnique = false
		}
		if other.From.Rank == m.From.Rank {
			rankUnique = false
		}
	}
	switch {
	case rivals == 0:
		return dst
	case fileUnique:
		return append(dst, byte('a'+m.From.File))
	case rankUnique:
		return append(dst, byte('1'+m.From.Rank))
	default:
		return appendSquare(dst, m.From)
	}
}
