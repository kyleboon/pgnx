package pipeline

import (
	"errors"
	"fmt"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
)

// Normalise makes a parsed game self-consistent the way pgn-extract's
// parser does (grammar.c check_result and consistent_FEN_tags), in place,
// before the game is replayed. It is the first stage of the pipeline and
// runs once per game, for every command, whether or not any criteria are
// in force.
//
// Result tag: "1/2" becomes "1/2-1/2"; when the movetext ends with a
// result token, an absent, empty or "?" tag is filled from it. A tag that
// disagrees with the token, or is not a PGN result at all, is left alone:
// the invalid-Result-becomes-"*" rule and the consistency checks belong
// to the criteria stage (apply.c play_moves), after tag criteria have
// seen the tag as written.
//
// SetUp/FEN tags: a game with a valid FEN tag but no SetUp tag silently
// gains SetUp "1" ("such a common problem that it makes more sense just
// to silently correct it"). SetUp "1" without a FEN tag, or an unparsable
// FEN, is inconsistent: Normalise returns a non-nil error and the game is
// Rejected, emitted nowhere, whatever else the run was told.
func Normalise(g *pgn.Game) error {
	normaliseResult(g)
	return consistentFENTags(g)
}

// normaliseResult is grammar.c check_result.
func normaliseResult(g *pgn.Game) {
	tag, ok := g.Tag("Result")
	if ok && tag == "1/2" {
		tag = "1/2-1/2"
		g.SetTag("Result", tag)
	}
	if g.Result != "" && (!ok || tag == "" || tag == "?") {
		g.SetTag("Result", g.Result)
	}
}

var errSetUpWithoutFEN = errors.New(`SetUp "1" tag without a FEN tag`)

// consistentFENTags is grammar.c consistent_FEN_tags, minus the Chess960
// Variant tag it also adds (beads pgnx-chess960-variant).
func consistentFENTags(g *pgn.Game) error {
	setup, hasSetup := g.Tag("SetUp")
	fen, hasFEN := g.Tag("FEN")
	if hasSetup && setup == "1" && !hasFEN {
		return errSetUpWithoutFEN
	}
	if hasFEN {
		if _, err := chess.FromFEN(fen); err != nil {
			return fmt.Errorf("invalid FEN tag %q: %w", fen, err)
		}
		if !hasSetup {
			g.SetTag("SetUp", "1")
		}
	}
	return nil
}
