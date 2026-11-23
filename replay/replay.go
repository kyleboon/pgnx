package replay

import (
	"fmt"
	"strings"
	"sync"

	"github.com/kyleboon/pgnx/chess"
	"github.com/kyleboon/pgnx/pgn"
)

// Options controls replay behavior. Chess960 castling is data-driven via
// the game's FEN tag rather than a flag here.
type Options struct {
	// AllowNullMoves accepts the null move "--" (pgn-extract also takes
	// the spelling "Z0") in the main line (--allownullmoves). Without it
	// a main-line null move is an illegal move, as apply.c play_moves
	// rejects the game ("Null moves (--) only allowed in variations");
	// inside a variation a null move is always accepted.
	AllowNullMoves bool
}

// Result is the outcome of replaying a sequence of moves (a main line or a
// variation) against a starting position.
//
// Applying Moves[i].Move to the board of Positions[i] yields Positions[i+1],
// so a caller that needs a board at every ply walks the typed moves from
// StartBoard; Result itself keeps only the first and last boards.
type Result struct {
	Moves     []MoveInfo // parallel to the source []*pgn.Move
	Positions []Position // len = len(Moves)+1; [0] is the starting position

	// variations holds the replays of each move's variations, indexed
	// by the move's position in Moves, parallel to the pgn.Move's
	// Variations; nil until a move of the line has one, since most games
	// have none and a slice header per ply would be the largest part of
	// a MoveInfo. Read through Variations.
	variations [][]*Result

	start, final *chess.Board // boards of Positions[0] and the last Position
}

// MoveInfo describes one replayed ply. The replays of the move's
// variations are Result.Variations(i), not a field, so a MoveInfo stays
// small: it is allocated once per ply of every game.
type MoveInfo struct {
	SAN       string // normalized, with + / # suffix
	Move      chess.Move
	Piece     chess.Piece
	Captured  chess.PieceType // NoPiece if quiet
	EnPassant bool            // a pawn capturing en passant (Captured is Pawn)
	IsCheck   bool
	IsMate    bool
}

// Variations returns the replays of the variations of Moves[i], parallel
// to the source pgn.Move's Variations: a variation that failed to replay
// is a nil *Result. It is nil when the move has no variations, when i is
// not an index of Moves, or for an empty Result.
func (r *Result) Variations(i int) []*Result {
	if r == nil || i < 0 || i >= len(r.Moves) || i >= len(r.variations) {
		return nil
	}
	return r.variations[i]
}

// Position is a single position reached during replay: the typed facts
// a per-ply consumer reads (hashes, clocks, side to move). It carries no
// FEN text; a consumer that needs the FEN of a position gets a board from
// Result.BoardAt, and one that needs every ply's FEN walks the typed
// Moves from StartBoard, as pgn-extract builds FEN components only when
// an output asks for them (apply.c).
type Position struct {
	Zobrist uint64

	// Placement is chess.Board.PlacementKey() for the position: a hash of
	// the piece placement only, equal across differences in side to move,
	// castling rights, en passant square and clocks. Positional matching
	// and ECO classification compare positions by this key.
	Placement uint64

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
	rp := replayers.Get().(*replayer)
	defer replayers.Put(rp)
	rp.reset()
	return rp.replayLine(g.Moves, b, opts.AllowNullMoves)
}

// replayers holds replayers between games, so the move buffer each ply
// generates into is made once per goroutine rather than once per game
// (it was the largest per-game allocation of a run). Game is called from
// several goroutines at once, so the state is pooled, never a package
// variable; a Result keeps no reference into a replayer.
var replayers = sync.Pool{New: func() any { return newReplayer() }}

// startingBoard returns the board g's replay should start from: the
// position described by its FEN tag if present, else the standard starting
// position.
func startingBoard(g *pgn.Game) (*chess.Board, error) {
	if fen, ok := g.Tag("FEN"); ok && fen != "" {
		b, err := chess.FromFEN(fen)
		if err != nil {
			return nil, fmt.Errorf("invalid FEN tag %q: %w", fen, err)
		}
		// A fullmove field below 1 counts as move 1 (apply.c
		// new_fen_board), so ply numbers and emitted FENs agree with
		// the C tool.
		if b.FullmoveNumber < 1 {
			b.FullmoveNumber = 1
		}
		return b, nil
	}
	return chess.NewBoard(), nil
}

// PlyNumber is the ply number of p: the absolute, one-based number of the
// move about to be played from it, as pgn-extract's play_moves counts
// (apply.c: plies = move_number*2 - (to_move == WHITE ? 1 : 0)). It is 1
// at the standard starting position; for a game that starts from a FEN
// tag it is derived from the FEN's fullmove number and side to move, so
// "... b - - 0 40" is ply 80. The number of plies completed on reaching p
// (apply.c half_moves_played) is PlyNumber()-1.
func (p Position) PlyNumber() int {
	n := 2*(p.FullmoveNumber-1) + 1
	if p.SideToMove == chess.Black {
		n++
	}
	return n
}

// replayer is the state one replay of a game shares across its plies and
// variations: a single buffer the moves of a position are generated into,
// which SAN decoding, SAN rendering and mate detection all read, so no
// ply generates more than once and no generation allocates once the
// buffer has grown. The buffer holds either every legal move of a
// position (mate detection after a check, castling text and bare
// long-algebraic notation need them all) or only the candidates a SAN
// move names, the side to move's pieces of one type that can reach one
// square, as pgn-extract's map.c finds them. The buffer is handed down to
// variation replays, which overwrite it; a line therefore reads it only
// before replaying a move's variations, and only the first of a move's
// variations finds the parent's list in it.
type replayer struct {
	buf []chess.Move

	// holds says what buf holds for the board the next ply is played
	// from: nothing of use, its legal moves, or the candidates of the
	// piece type and destination in target.
	holds  bufferContents
	target moveTarget

	// full counts calls of the full legal-move generator and targeted
	// calls of the candidate generator, the costs
	// TestReplayGeneratesLegalMovesOncePerPosition bounds per ply.
	full, targeted int
}

// bufferContents says what a replayer's buffer holds for the position
// the next ply is played from.
type bufferContents uint8

const (
	holdsStale    bufferContents = iota // some other position's moves
	holdsLegal                          // every legal move
	holdsTargeted                       // the candidates of one moveTarget
)

// moveTarget is the piece type and destination square a SAN move names.
type moveTarget struct {
	piece chess.PieceType
	to    chess.Square
}

func newReplayer() *replayer {
	return &replayer{buf: make([]chess.Move, 0, moveBufferHint)}
}

// reset readies a replayer taken from the pool for a new game: its
// buffer holds another game's moves and its counters another game's
// generations. The buffer's capacity is kept.
func (rp *replayer) reset() {
	rp.holds = holdsStale
	rp.full, rp.targeted = 0, 0
}

// moveBufferHint is the initial capacity of a replayer's move buffer:
// enough for the pseudo-legal moves of nearly every position reached in
// play, so the buffer is rarely regrown.
const moveBufferHint = 128

// legalMoves returns the legal moves of b, generating them into the shared
// buffer unless it already holds them.
func (rp *replayer) legalMoves(b *chess.Board) []chess.Move {
	if rp.holds != holdsLegal {
		rp.buf = b.AppendLegalMoves(rp.buf[:0])
		rp.full++
		rp.holds = holdsLegal
	}
	return rp.buf
}

// candidates returns legal moves of b that include every move of a piece
// of type pt to to: the position's legal moves when the buffer already
// holds them, else only those candidates, generated into the buffer unless
// it holds them from the ply's decoding. Callers filter by piece type and
// destination, as the list may be the wider one.
func (rp *replayer) candidates(b *chess.Board, pt chess.PieceType, to chess.Square) []chess.Move {
	if rp.holds == holdsLegal {
		return rp.buf
	}
	t := moveTarget{piece: pt, to: to}
	if rp.holds != holdsTargeted || rp.target != t {
		rp.buf = b.AppendLegalMovesTo(pt, to, rp.buf[:0])
		rp.targeted++
		rp.holds = holdsTargeted
		rp.target = t
	}
	return rp.buf
}

// invalidate records that the buffer no longer describes the board the
// next ply is played from.
func (rp *replayer) invalidate() {
	rp.holds = holdsStale
}

// replayLine replays moves in sequence against b (which is mutated in place),
// recording a MoveInfo and Position per move plus the leading Position for
// b's starting state. Variations attached to each move are replayed
// recursively from a copy of the position before that move; a variation
// whose own replay fails does not fail the parent replay — it is recorded
// as a nil *Result. The first illegal move in moves itself stops replay
// and returns the partial Result together with an *IllegalMoveError whose
// Ply is the 1-based index of the failing move within moves. A null move
// is illegal in moves unless allowNull; variations always allow it.
func (rp *replayer) replayLine(moves []*pgn.Move, b *chess.Board, allowNull bool) (*Result, error) {
	r := &Result{
		Moves:     make([]MoveInfo, 0, len(moves)),
		Positions: make([]Position, 0, len(moves)+1),
		start:     b.Copy(),
		final:     b, // mutated below; the last position when replay stops
	}
	r.Positions = append(r.Positions, positionOf(b))

	// The SAN of every move of the line is cut from one text, grown once
	// up front for the usual move length, so a game costs one string
	// allocation rather than one per ply. A Builder never rewrites what
	// it has already written, so the SAN strings sliced from its text
	// stay valid when it grows.
	var sanText strings.Builder
	sanText.Grow(len(moves) * sanLengthHint)

	for i, mv := range moves {
		var (
			m         chess.Move
			piece     chess.Piece
			captured  chess.PieceType
			enPassant bool
			// The move's SAN, built on the stack and turned into a
			// string once its check suffix is known.
			sanBuf [16]byte
			san    = sanBuf[:0]
		)
		if isNullMove(mv.SAN) {
			if !allowNull {
				return r, &IllegalMoveError{
					Ply:    i + 1,
					SAN:    mv.SAN,
					Reason: "null move (--) only allowed in variations without --allow-null-moves",
				}
			}
			m = chess.NullMove
			san = append(san, NullMoveSAN...)
		} else {
			var err error
			m, err = decodeSAN(mv.SAN, b, rp)
			if err != nil {
				return r, &IllegalMoveError{
					Ply:    i + 1,
					SAN:    mv.SAN,
					Reason: err.Error(),
				}
			}
			san = appendSAN(san, m, b, rp)
			piece = b.PieceAt(m.From)
			captured, enPassant = capturedPieceType(m, b)
		}

		// Variations replay from the position before this move, so branch
		// off a copy of b prior to applying m. They share the move buffer:
		// the first starts with this position's moves in it, and each one
		// leaves some other position's moves behind.
		if vars := mv.Variations(); len(vars) > 0 {
			variations := make([]*Result, len(vars))
			for vi, v := range vars {
				vr, verr := rp.replayLine(v.Moves, b.Copy(), true)
				rp.invalidate()
				if verr != nil {
					variations[vi] = nil
					continue
				}
				variations[vi] = vr
			}
			if r.variations == nil {
				r.variations = make([][]*Result, len(moves))
			}
			r.variations[i] = variations
		}

		b.Apply(m)
		rp.invalidate()

		// As apply.c play_moves: a check test first, and only a checking
		// move is tested for mate. The mate test stops at the checked
		// side's first legal reply rather than generating them all: in
		// check most candidates are illegal and each costs a full
		// legality probe, far more than the one targeted generation the
		// next ply pays for not finding the list in the buffer.
		isCheck := b.InCheck(b.SideToMove)
		isMate := isCheck && !b.HasLegalMove()
		if isMate {
			san = append(san, '#')
		} else if isCheck {
			san = append(san, '+')
		}

		off := sanText.Len()
		sanText.Write(san)
		r.Moves = append(r.Moves, MoveInfo{
			SAN:       sanText.String()[off:],
			Move:      m,
			Piece:     piece,
			Captured:  captured,
			EnPassant: enPassant,
			IsCheck:   isCheck,
			IsMate:    isMate,
		})
		r.Positions = append(r.Positions, positionOf(b))
	}

	return r, nil
}

// sanLengthHint is the text a line reserves per move for its SAN: most
// moves are three or four characters ("Nf3", "exd5", "O-O"), a check or a
// capture by a named piece five, so the text rarely has to grow.
const sanLengthHint = 5

// NullMoveSAN is how pgn-extract writes a null move in every format
// (defs.h NULL_MOVE_STRING); it is the SAN of a null MoveInfo.
const NullMoveSAN = "--"

// isNullMove reports whether san, as written, is a null move: "--" or
// pgn-extract's alternative spelling "Z0" (lex.c), either with trailing
// annotation glyphs, which the C parser may append and the output drops.
func isNullMove(san string) bool {
	switch trimAnnotation(san) {
	case NullMoveSAN, "Z0":
		return true
	}
	return false
}

// capturedPieceType returns the type of piece m captures on b, before m is
// applied, or chess.NoPiece if m is not a capture, and whether the capture
// is en passant: a pawn moving to b's en passant square takes the pawn
// beside m.To rather than the (empty) square itself.
func capturedPieceType(m chess.Move, b *chess.Board) (chess.PieceType, bool) {
	mover := b.PieceAt(m.From)
	if mover.Type == chess.Pawn && b.HasEP && m.To == b.EPSquare {
		return chess.Pawn, true
	}
	return b.PieceAt(m.To).Type, false
}

// positionOf snapshots b's Zobrist hash, placement key and clocks.
func positionOf(b *chess.Board) Position {
	return Position{
		Zobrist:        b.Zobrist(),
		Placement:      b.PlacementKey(),
		HalfmoveClock:  b.HalfmoveClock,
		FullmoveNumber: b.FullmoveNumber,
		SideToMove:     b.SideToMove,
	}
}

// PlyCount returns the game's length in plies as pgn-extract defines it
// (apply.c apply_move_list): the plies completed on reaching the final
// position, one less than its PlyNumber, not a count of moves played. For
// a game that starts from a FEN tag this is therefore the absolute
// half-move number reached, which is what the C tool's -b/-p bounds
// compare against. It is 0 for an empty Result.
func (r *Result) PlyCount() int {
	if r == nil || len(r.Positions) == 0 {
		return 0
	}
	return r.Positions[len(r.Positions)-1].PlyNumber() - 1
}

// StartBoard returns a copy of the board of Positions[0], the caller's to
// mutate: applying the typed Moves to it in order walks the line's
// positions. It is nil for an empty Result.
func (r *Result) StartBoard() *chess.Board {
	if r == nil || r.start == nil {
		return nil
	}
	return r.start.Copy()
}

// BoardAt returns a copy of the board of Positions[i], the caller's to
// mutate: the final board when i is the last position, else the start
// board with the first i typed Moves applied. It is nil when i is out of
// range or the Result is empty. A caller that needs the board of every
// position walks StartBoard itself rather than calling BoardAt per ply.
func (r *Result) BoardAt(i int) *chess.Board {
	if r == nil || i < 0 || i >= len(r.Positions) {
		return nil
	}
	if i == len(r.Positions)-1 {
		return r.FinalBoard()
	}
	b := r.StartBoard()
	for _, m := range r.Moves[:i] {
		b.Apply(m.Move)
	}
	return b
}

// FinalBoard returns a copy of the board of the last Position reached (for
// a partial Result, the position the failing move was attempted from), for
// predicates that need the board itself (stalemate, insufficient
// material). It is nil for an empty Result.
func (r *Result) FinalBoard() *chess.Board {
	if r == nil || r.final == nil {
		return nil
	}
	return r.final.Copy()
}
