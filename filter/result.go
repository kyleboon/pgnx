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
		g.SetTag("Result", "*")
	}
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
			if b := r.FinalBoard(); b != nil && (b.IsStalemate() || b.InsufficientMaterial()) {
				corrected = "1/2-1/2"
			}
		}
		if corrected != "" {
			g.SetTag("Result", corrected)
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
			g.SetTag("Result", moveResult)
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
