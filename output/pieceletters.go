package output

import (
	"fmt"

	"github.com/kyleboon/pgnx/chess"
)

// englishPieceLetters is the C oracle's default output_piece_characters
// (apply.c): the letters SAN, ELAlg and XLAlg spell pieces with, and the
// promotion letter of every long-algebraic-family format, UCI included.
// Promotion letters are uppercase, matching piece_str() — not the lowercase
// convention of the standalone UCI protocol / chess.Move.UCI().
var englishPieceLetters = [chess.King + 1]string{
	chess.Pawn:   "P",
	chess.Knight: "N",
	chess.Bishop: "B",
	chess.Rook:   "R",
	chess.Queen:  "Q",
	chess.King:   "K",
}

// PieceLetters is the alphabet the SAN, ELAlg and XLAlg formats spell
// pieces with: pgn-extract's -Wsan<letters> / -Welalg<letters> /
// -Wxlalg<letters> suffix (apply.c set_output_piece_characters). The zero
// value is the default English alphabet (PNBRQK). It only affects output:
// replay reads moves with its fixed letters (English plus the German
// D/T/S/L, as decode.c does) whatever alphabet the output uses.
type PieceLetters struct {
	byType [chess.King + 1]string
}

// ParsePieceLetters parses a piece-letter string: six letters in the order
// pawn, knight, bishop, rook, queen, king, each either one character or
// the two-character form "X+Y" for a piece spelled "XY" (so "P+pN+n..."
// spells a knight "Nn"). The C tool works on bytes; ParsePieceLetters
// counts runes, so a multi-byte letter (a Cyrillic "Кр" is "К+р") counts
// as one.
func ParsePieceLetters(s string) (PieceLetters, error) {
	var p PieceLetters
	runes := []rune(s)
	i := 0
	for pt := chess.Pawn; pt <= chess.King; pt++ {
		if i >= len(runes) {
			return PieceLetters{}, fmt.Errorf("output: insufficient piece letters in %q: six are needed, in the order PNBRQK", s)
		}
		letter := string(runes[i])
		i++
		if i < len(runes) && runes[i] == '+' {
			i++
			if i >= len(runes) {
				return PieceLetters{}, fmt.Errorf("output: missing piece letter following + in %q", s)
			}
			letter += string(runes[i])
			i++
		}
		p.byType[pt] = letter
	}
	if i < len(runes) {
		return PieceLetters{}, fmt.Errorf("output: too many piece letters in %q: six are needed, in the order PNBRQK", s)
	}
	return p, nil
}

// Letter returns the letter pt is spelled with: p's, or the English
// default when p is the zero value.
func (p PieceLetters) Letter(pt chess.PieceType) string {
	if p.byType[pt] == "" {
		return englishPieceLetters[pt]
	}
	return p.byType[pt]
}
