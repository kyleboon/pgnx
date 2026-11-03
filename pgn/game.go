package pgn

import "slices"

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
	// Annotations are the tokens that followed the move in the source,
	// in the order they were written: each is a NAG, a Comment or a
	// *Variation. A comment written after a variation's closing paren is
	// an annotation of this move, following that variation, as
	// pgn-extract reads it. NAGs, Comments and Variations are views of
	// this list; AddComment is how the rest of pgnx adds to it.
	Annotations []Annotation
}

// Annotation is one token following a move: a NAG, a Comment or a
// *Variation. The three types are the only implementations.
type Annotation interface {
	annotation()
}

// NAG is a numeric annotation glyph, the N of "$N".
type NAG int

// Comment is the text of a brace comment, without the braces.
type Comment string

func (NAG) annotation()        {}
func (Comment) annotation()    {}
func (*Variation) annotation() {}

// NAGs returns the move's NAGs in source order; nil when it has none.
func (m *Move) NAGs() []int {
	var nags []int
	for _, a := range m.Annotations {
		if n, ok := a.(NAG); ok {
			nags = append(nags, int(n))
		}
	}
	return nags
}

// Comments returns every comment on the move in source order, wherever it
// sits among the NAGs and variations; nil when there are none.
func (m *Move) Comments() []string {
	var comments []string
	for _, a := range m.Annotations {
		if c, ok := a.(Comment); ok {
			comments = append(comments, string(c))
		}
	}
	return comments
}

// LeadingComments returns the comments written directly after the move,
// before its first NAG or variation: pgn-extract's comment_list, the only
// comments its --commented, --dropbefore and comment-matching options
// look at. nil when there are none.
func (m *Move) LeadingComments() []string {
	var comments []string
	for _, a := range m.Annotations {
		c, ok := a.(Comment)
		if !ok {
			break
		}
		comments = append(comments, string(c))
	}
	return comments
}

// Variations returns the move's variations in source order; nil when it
// has none.
func (m *Move) Variations() []*Variation {
	var variations []*Variation
	for _, a := range m.Annotations {
		if v, ok := a.(*Variation); ok {
			variations = append(variations, v)
		}
	}
	return variations
}

// AddComment adds a comment to the move after its leading comments and
// before its first NAG or variation, where pgn-extract's
// append_comments_to_move puts the comments it generates (a
// --mark-matches comment on "e4 $1" is emitted as "e4 { mark } $1").
func (m *Move) AddComment(text string) {
	i := 0
	for i < len(m.Annotations) {
		if _, ok := m.Annotations[i].(Comment); !ok {
			break
		}
		i++
	}
	m.Annotations = slices.Insert(m.Annotations, i, Annotation(Comment(text)))
}

// Variation is a recursive annotation variation (RAV): an alternative line
// attached to a move.
type Variation struct {
	PrefixComments []string
	Moves          []*Move
	// Result is a result token written inside the variation, before its
	// closing paren ("(1... c5 1-0)"), which pgn-extract keeps and emits
	// after the variation's last move; "" when there is none.
	Result string
}
