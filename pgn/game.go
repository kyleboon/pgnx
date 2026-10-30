package pgn

// Tag is a single PGN header tag, e.g. [Event "Foo"].
type Tag struct {
	Name  string
	Value string
}

// Game is a single parsed PGN game.
type Game struct {
	Tags           []Tag // in appearance order; duplicates kept, last-wins on lookup
	PrefixComments []string
	Moves          []*Move // main line
	Result         string  // "1-0", "0-1", "1/2-1/2", "*" ("" if missing)
	StartLine      int
	EndLine        int

	errs    []error
	skipped []error
}

// Tag returns the value of the last tag named name, and whether it was
// found. If the tag appears more than once, the last occurrence wins.
func (g *Game) Tag(name string) (string, bool) {
	value, found := "", false
	for _, t := range g.Tags {
		if t.Name == name {
			value, found = t.Value, true
		}
	}
	return value, found
}

// ParseErrors returns the recoverable errors collected while parsing this
// game. Parsing a game never fails hard; problems such as a missing result
// or an unparsable NAG are recorded here instead.
func (g *Game) ParseErrors() []error {
	return g.errs
}

// SkippedTokens returns the movetext tokens that were not moves (a lone
// "-", a bare piece letter, ...) and were discarded while parsing this
// game, as pgn-extract does, one error per token. The game itself is
// otherwise intact.
func (g *Game) SkippedTokens() []error {
	return g.skipped
}

// Move is a single ply of movetext.
type Move struct {
	SAN        string // exactly as written ("Nf3!?")
	MoveNumber int    // 0 if none was written before this move
	NAGs       []int
	Comments   []string // comments following this move
	Variations []*Variation
}

// Variation is a recursive annotation variation (RAV): an alternative line
// attached to a move.
type Variation struct {
	PrefixComments []string
	Moves          []*Move
}
