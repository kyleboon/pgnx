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

// SetTag sets the tag named name to value. If the tag is present its last
// occurrence (the one Tag reads) is replaced in place, so the game's tag
// order is preserved; otherwise the tag is appended. SetTag and DeleteTag
// are the only ways the rest of pgnx mutates a parsed game's tags.
func (g *Game) SetTag(name, value string) {
	for i := len(g.Tags) - 1; i >= 0; i-- {
		if g.Tags[i].Name == name {
			g.Tags[i].Value = value
			return
		}
	}
	g.Tags = append(g.Tags, Tag{Name: name, Value: value})
}

// DeleteTag removes every tag named name, keeping the order of the rest.
// A name the game lacks is a no-op.
func (g *Game) DeleteTag(name string) {
	kept := g.Tags[:0]
	for _, t := range g.Tags {
		if t.Name != name {
			kept = append(kept, t)
		}
	}
	g.Tags = kept
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
