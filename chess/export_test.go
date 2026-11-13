package chess

// Hash tables exported to the package's external tests, which compute
// the placement key and weak hash of the mailbox oracle by scanning its
// squares and compare them with the bitboard Board's maintained values.
var (
	PolyglotRandom = polyglotRandom
	WeakTable      = &weakTable // a pointer: the table is filled by init, after package variables
)
