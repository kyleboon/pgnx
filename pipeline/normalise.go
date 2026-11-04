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

// chess960Variant is the Variant tag value the C tool gives a Chess960
// setup it finds untagged.
const chess960Variant = "chess 960"

// consistentFENTags is grammar.c consistent_FEN_tags: SetUp "1" without
// a FEN tag, or a FEN that does not parse, rejects the game; a FEN
// without SetUp gets SetUp "1"; and a FEN game with no Variant tag
// whose start is a Chess960 setup (chess960Setup) gets
// [Variant "chess 960"]. The C tool logs the added Variant; pgnx adds it
// silently, as it adds SetUp. (The C tool's --addfencastling branch is
// not ported.)
func consistentFENTags(g *pgn.Game) error {
	setup, hasSetup := g.Tag("SetUp")
	fen, hasFEN := g.Tag("FEN")
	if hasSetup && setup == "1" && !hasFEN {
		return errSetUpWithoutFEN
	}
	if hasFEN {
		b, err := chess.FromFEN(fen)
		if err != nil {
			return fmt.Errorf("invalid FEN tag %q: %w", fen, err)
		}
		if !hasSetup {
			g.SetTag("SetUp", "1")
		}
		if _, hasVariant := g.Tag("Variant"); !hasVariant && chess960Setup(b) {
			g.SetTag("Variant", chess960Variant)
		}
	}
	return nil
}

// standardBackRank is White's back rank in the standard start.
var standardBackRank = [8]chess.PieceType{chess.Rook, chess.Knight, chess.Bishop, chess.Queen, chess.King, chess.Bishop, chess.Knight, chess.Rook}

// chess960Setup is grammar.c chess960_setup: whether b looks like the
// initial position of a Chess960 game. That takes move number 1, both
// kings on their home rank on the same file, all four castling rights,
// a full rank of pawns for each side, a full back rank with the same
// piece type on each file for both sides, and White's back rank not the
// standard RNBQKBNR (compared as coloured pieces, as the C tool does, so
// a black piece on White's back rank counts as off standard). Side to
// move is not consulted.
func chess960Setup(b *chess.Board) bool {
	if b.FullmoveNumber != 1 {
		return false
	}
	wk, bk := b.KingSquare(chess.White), b.KingSquare(chess.Black)
	if wk.Rank != 0 || bk.Rank != 7 || wk.File != bk.File {
		return false
	}
	c := b.Castling
	if !c.WhiteKingside || !c.WhiteQueenside || !c.BlackKingside || !c.BlackQueenside {
		return false
	}
	offStandard := false
	for f := 0; f < 8; f++ {
		if b.PieceAt(chess.Square{File: f, Rank: 1}) != (chess.Piece{Type: chess.Pawn, Color: chess.White}) ||
			b.PieceAt(chess.Square{File: f, Rank: 6}) != (chess.Piece{Type: chess.Pawn, Color: chess.Black}) {
			return false
		}
		white := b.PieceAt(chess.Square{File: f, Rank: 0})
		if white.Type == chess.NoPiece || white.Type != b.PieceAt(chess.Square{File: f, Rank: 7}).Type {
			return false
		}
		if white != (chess.Piece{Type: standardBackRank[f], Color: chess.White}) {
			offStandard = true
		}
	}
	return offStandard
}
