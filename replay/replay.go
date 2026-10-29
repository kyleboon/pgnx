package replay

import (
	"fmt"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
)

// Options controls replay behavior. It is currently reserved: Chess960
// castling is data-driven via the game's FEN tag rather than a flag here.
type Options struct{}

// Result is the outcome of replaying a sequence of moves (a main line or a
// variation) against a starting position.
type Result struct {
	Moves     []MoveInfo // parallel to the source []*pgn.Move
	Positions []Position // len = len(Moves)+1; [0] is the starting position
}

// MoveInfo describes one replayed ply.
type MoveInfo struct {
	SAN        string // normalized, with + / # suffix
	Move       chess.Move
	Piece      chess.Piece
	Captured   chess.PieceType // NoPiece if quiet
	IsCheck    bool
	IsMate     bool
	Variations []*Result // parallel to the pgn.Move's Variations
}

// Position is a single position reached during replay.
type Position struct {
	FEN     string
	Zobrist uint64

	// HalfmoveClock, FullmoveNumber, and SideToMove are the FEN's clock
	// fields and side to move, exposed so that callers (the filter
	// package's fifty-move and length predicates) need not re-parse FEN.
	HalfmoveClock  int
	FullmoveNumber int
	SideToMove     chess.Color
}

// IllegalMoveError reports that a move in the game record could not be
// legally applied to the position it was played from.
type IllegalMoveError struct {
	Ply    int // 1-based half-move index
	SAN    string
	Reason string
}

func (e *IllegalMoveError) Error() string {
	return fmt.Sprintf("illegal move %d (%s): %s", e.Ply, e.SAN, e.Reason)
}

// Game replays g's main line (and, recursively, its variations) move by
// move, returning the resulting positions and per-move detail.
//
// The initial position is taken from g's FEN tag if present, else the
// standard starting position. If the first illegal main-line move is
// encountered, Game returns a partial Result (the moves and positions up
// to, but not including, the failure) together with an *IllegalMoveError.
func Game(g *pgn.Game, opts Options) (*Result, error) {
	b, err := startingBoard(g)
	if err != nil {
		return nil, err
	}
	return replayMoves(g.Moves, b)
}

// startingBoard returns the board g's replay should start from: the
// position described by its FEN tag if present, else the standard starting
// position.
func startingBoard(g *pgn.Game) (*chess.Board, error) {
	if fen, ok := g.Tag("FEN"); ok && fen != "" {
		b, err := chess.FromFEN(fen)
		if err != nil {
			return nil, fmt.Errorf("invalid FEN tag %q: %w", fen, err)
		}
		return b, nil
	}
	return chess.NewBoard(), nil
}

// replayMoves replays moves in sequence against b (which is mutated in
// place), recording a MoveInfo and Position per move plus the leading
// Position for b's starting state. Variations attached to each move are
// replayed recursively from a copy of the position before that move; a
// variation whose own replay fails does not fail the parent replay — it is
// recorded as a nil *Result. The first illegal move in moves itself stops
// replay and returns the partial Result together with an *IllegalMoveError
// whose Ply is the 1-based index of the failing move within moves.
func replayMoves(moves []*pgn.Move, b *chess.Board) (*Result, error) {
	r := &Result{
		Positions: make([]Position, 0, len(moves)+1),
	}
	r.Positions = append(r.Positions, positionOf(b))

	for i, mv := range moves {
		m, err := decodeSAN(mv.SAN, b)
		if err != nil {
			return r, &IllegalMoveError{
				Ply:    i + 1,
				SAN:    mv.SAN,
				Reason: err.Error(),
			}
		}

		san := sanFor(m, b)
		piece := b.PieceAt(m.From)
		captured := capturedPieceType(m, b)

		// Variations replay from the position before this move, so branch
		// off a copy of b prior to applying m.
		var variations []*Result
		if len(mv.Variations) > 0 {
			variations = make([]*Result, len(mv.Variations))
			for vi, v := range mv.Variations {
				vb := b.Copy()
				vr, verr := replayMoves(v.Moves, vb)
				if verr != nil {
					variations[vi] = nil
					continue
				}
				variations[vi] = vr
			}
		}

		b.Apply(m)

		sideToMove := b.SideToMove
		isMate := b.IsCheckmate()
		isCheck := isMate || b.InCheck(sideToMove)
		if isMate {
			san += "#"
		} else if isCheck {
			san += "+"
		}

		r.Moves = append(r.Moves, MoveInfo{
			SAN:        san,
			Move:       m,
			Piece:      piece,
			Captured:   captured,
			IsCheck:    isCheck,
			IsMate:     isMate,
			Variations: variations,
		})
		r.Positions = append(r.Positions, positionOf(b))
	}

	return r, nil
}

// capturedPieceType returns the type of piece m captures on b, before m is
// applied, or chess.NoPiece if m is not a capture. En passant captures the
// pawn beside m.To rather than the piece (if any) on m.To.
func capturedPieceType(m chess.Move, b *chess.Board) chess.PieceType {
	mover := b.PieceAt(m.From)
	if mover.Type == chess.Pawn && b.HasEP && m.To == b.EPSquare {
		return chess.Pawn
	}
	return b.PieceAt(m.To).Type
}

// positionOf snapshots b's current FEN and Zobrist hash.
func positionOf(b *chess.Board) Position {
	return Position{
		FEN:            b.FEN(),
		Zobrist:        b.Zobrist(),
		HalfmoveClock:  b.HalfmoveClock,
		FullmoveNumber: b.FullmoveNumber,
		SideToMove:     b.SideToMove,
	}
}

// PlyCount returns the game's length in plies as pgn-extract defines it
// (apply.c apply_move_list): derived from the final position's fullmove
// number and side to move, not from a count of moves played. For a game
// that starts from a FEN tag this is therefore the absolute half-move
// number reached, which is what the C tool's -b/-p bounds compare
// against. It is 0 for an empty Result.
func (r *Result) PlyCount() int {
	if r == nil || len(r.Positions) == 0 {
		return 0
	}
	last := r.Positions[len(r.Positions)-1]
	if last.SideToMove == chess.Black {
		return 2*(last.FullmoveNumber-1) + 1
	}
	return 2 * (last.FullmoveNumber - 1)
}

// FinalBoard reconstructs the last position reached from its FEN. It is a
// convenience for predicates that need the board itself (stalemate,
// insufficient material); Result does not retain a Board per ply.
func (r *Result) FinalBoard() (*chess.Board, error) {
	if r == nil || len(r.Positions) == 0 {
		return nil, fmt.Errorf("replay: empty result has no final position")
	}
	return chess.FromFEN(r.Positions[len(r.Positions)-1].FEN)
}
