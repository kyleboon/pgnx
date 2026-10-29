package filter

import (
	"errors"
	"strings"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// ErrBadResult is returned by Evaluate when Options.NoBadResults is set
// and the game's Result tag disagrees with its terminating result token.
// The CLI treats such a game like a broken one: logged, emitted nowhere,
// exit status 2.
var ErrBadResult = errors.New("filter: result tag disagrees with the game's result")

// normaliseResult is pgn-extract's check_result (grammar.c), which its
// parser applies to every game before any matching: a Result tag of
// "1/2" becomes "1/2-1/2", and when the movetext ends with a result
// token, an absent, empty or "?" Result tag is filled from it.
func normaliseResult(g *pgn.Game) {
	tag, ok := g.Tag("Result")
	if ok && tag == "1/2" {
		tag = "1/2-1/2"
		setTag(g, "Result", tag)
	}
	if g.Result != "" && (!ok || tag == "" || tag == "?") {
		setTag(g, "Result", g.Result)
	}
}

// validResult is apply.c valid_result.
func validResult(s string) bool {
	switch s {
	case "1-0", "0-1", "1/2-1/2", "*":
		return true
	}
	return false
}

// ensureValidResultTag is the start of apply.c play_moves: a Result tag
// that is not one of the four PGN results is replaced by "*".
func ensureValidResultTag(g *pgn.Game) {
	if tag, ok := g.Tag("Result"); ok && !validResult(tag) {
		setTag(g, "Result", "*")
	}
}

// consistentFENTags is grammar.c consistent_FEN_tags: a game with a
// valid FEN tag but no SetUp tag silently gains SetUp "1"; SetUp "1"
// without a FEN tag, or an unparsable FEN, makes the game inconsistent
// (it is then rejected by Evaluate).
func consistentFENTags(g *pgn.Game) bool {
	consistent := true
	setup, hasSetup := g.Tag("SetUp")
	fen, hasFEN := g.Tag("FEN")
	if hasSetup && setup == "1" && !hasFEN {
		consistent = false
	}
	if hasFEN {
		if _, err := chess.FromFEN(fen); err != nil {
			consistent = false
		} else if !hasSetup {
			setTag(g, "SetUp", "1")
		}
	}
	return consistent
}

// checkResult is the end-of-game result consistency check in apply.c
// play_moves, run only when the whole main line replayed. With fix set
// (--fixresulttags): a final checkmate forces 1-0/0-1 for the mating
// side; otherwise a non-draw tag with a final stalemate or insufficient
// material becomes 1/2-1/2 (both rewrite the tag and the terminating
// token); then a tag that still differs from the token is replaced by
// the token when either of them is "*". With reject set (--nobadresults)
// a remaining disagreement, other than a "*" token, returns
// ErrBadResult. (Mate/stalemate disagreements only warn in C when not
// fixing; they never reject.)
func checkResult(g *pgn.Game, r *replay.Result, fix, reject bool) error {
	n := len(r.Moves)
	if n == 0 || n != len(g.Moves) || n+1 > len(r.Positions) {
		return nil
	}
	if tag, ok := g.Tag("Result"); ok {
		last := r.Moves[n-1]
		corrected := ""
		if last.IsMate {
			if r.Positions[n].SideToMove == chess.Black {
				if !strings.HasPrefix(tag, "1-0") && fix {
					corrected = "1-0"
				}
			} else if !strings.HasPrefix(tag, "0-1") && fix {
				corrected = "0-1"
			}
		} else if !strings.HasPrefix(tag, "1/2") && fix {
			if b, err := r.FinalBoard(); err == nil && (b.IsStalemate() || b.InsufficientMaterial()) {
				corrected = "1/2-1/2"
			}
		}
		if corrected != "" {
			setTag(g, "Result", corrected)
			g.Result = corrected
		}
	}

	tag, ok := g.Tag("Result")
	moveResult := g.Result
	if !ok || moveResult == "" || tag == moveResult {
		return nil
	}
	report := true
	if fix {
		if moveResult == "*" || tag == "*" {
			setTag(g, "Result", moveResult)
			report = false
		}
	} else if moveResult == "*" {
		report = false
	}
	if report && reject {
		return ErrBadResult
	}
	return nil
}
