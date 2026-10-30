package pgn

// moveSeemsValid reports whether s has the overall shape of a SAN-like
// move. It is a port of move_seems_valid (decode.c) in the C oracle, which
// lex.c uses to decide whether an alphabetic word is a move at all: a word
// that fails the test (a bare piece letter such as "N", a word that is not
// a square, "-O", "resigns", ...) is reported and discarded by the lexer
// rather than being handed to the parser as a move. It checks shape only,
// never legality. s must not include trailing '!'/'?' glyphs (the C lexer
// turns those into NAGs before the check ever sees them). The Russian
// piece letters of the C table are not supported, as pgnx's lexer does not
// accept those characters in movetext at all.
func moveSeemsValid(s string) bool {
	// at returns s[i], or 0 past the end, mirroring the NUL terminator the
	// C code relies on.
	at := func(i int) byte {
		if i < len(s) {
			return s[i]
		}
		return 0
	}
	isCol := func(c byte) bool { return c >= 'a' && c <= 'h' }
	isRank := func(c byte) bool { return c >= '1' && c <= '8' }
	isCapture := func(c byte) bool { return c == 'x' || c == 'X' || c == ':' || c == '-' }
	isCastle := func(c byte) bool { return c == 'O' || c == '0' || c == 'o' }
	isPiece := func(c byte) bool {
		switch c {
		case 'K', 'k', 'Q', 'q', 'D', 'R', 'r', 'T', 'N', 'n', 'P', 'S', 'B', 'L':
			return true
		}
		return false
	}

	i := 0
	ok := true
	pawn := false
	c := at(i)
	switch {
	case isCol(c):
		pawn = true
		i++
		if isRank(at(i)) {
			i++
			if isCapture(at(i)) {
				i++
			}
			if isCol(at(i)) {
				i++
				if isRank(at(i)) {
					i++
				}
			}
		} else {
			if isCapture(at(i)) {
				i++
			}
			if isCol(at(i)) {
				i++
				if isRank(at(i)) {
					i++
				}
			} else {
				ok = false
			}
		}
		if ok {
			if at(i) == '=' {
				i++
			}
			if isPiece(at(i)) || at(i) == 'b' {
				pawn = false // promotion
				i++
			}
		}
	case isPiece(c):
		i++
		if isRank(at(i)) {
			i++
			if isCapture(at(i)) {
				i++
			}
			if isCol(at(i)) {
				i++
				if isRank(at(i)) {
					i++
				}
			} else {
				ok = false
			}
		} else if isCapture(at(i)) {
			i++
			if isCol(at(i)) {
				i++
				if isRank(at(i)) {
					i++
				} else {
					ok = false
				}
			} else {
				ok = false
			}
		} else if isCol(at(i)) {
			i++
			if isCapture(at(i)) {
				i++
			}
			if isRank(at(i)) {
				i++
				if isCapture(at(i)) {
					i++
				}
				if isCol(at(i)) {
					i++
					if isRank(at(i)) {
						i++
					} else {
						ok = false
					}
				}
			} else if isCol(at(i)) {
				i++
				if isRank(at(i)) {
					i++
				} else {
					ok = false
				}
			} else {
				ok = false
			}
		} else {
			ok = false
		}
	case isCastle(c):
		i++
		if at(i) == '-' {
			i++
		}
		if isCastle(at(i)) {
			i++
			if at(i) == '-' {
				i++
			}
			if isCastle(at(i)) {
				i++
			}
		} else {
			ok = false
		}
	default:
		ok = false
	}
	if !ok {
		return false
	}
	for at(i) == '+' || at(i) == '#' {
		i++
	}
	rest := s[min(i, len(s)):]
	return rest == "" || (pawn && (rest == "ep" || rest == "e.p."))
}
