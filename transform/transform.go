package transform

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/kyleboon/pgnx/pgn"
	"github.com/kyleboon/pgnx/replay"
)

// Options selects the transforms Apply runs. The zero value runs only the
// rewrite pgn-extract always does: a HashCode tag the game already
// carries is recomputed.
type Options struct {
	// DropPlies drops the first N main-line plies (--dropply N) and
	// records the position reached as the game's FEN and SetUp tags. A
	// negative N keeps only the last -N plies. 0 is off.
	DropPlies int

	// DropBefore drops every main-line ply up to and including the move
	// carrying a comment equal to it, whitespace-trimmed (--dropbefore).
	// It overrides DropPlies when a comment matches. "" is off.
	DropBefore string

	// PlyCount adds a PlyCount tag: the number of main-line plies, after
	// any dropping (--plycount).
	PlyCount bool

	// TotalPlyCount adds a TotalPlyCount tag: the main-line plies plus,
	// when CountVariations, every variation ply (--totalplycount).
	TotalPlyCount bool

	// CountVariations makes TotalPlyCount count variation plies: the C
	// tool counts them unless variations are being dropped (-V).
	CountVariations bool

	// HashCode adds a HashCode tag: the low 32 bits of the sum of the
	// weak hashes of every main-line position after a move, in hex
	// (--addhashcode). The sum runs over the game before any dropping.
	HashCode bool

	// HashCodeReplay, when non-nil, is the replay HashCode is summed over
	// instead of the game's own: the main line's, for the games split from
	// it. The C tool computes cumulative_hash_value once, when the game is
	// replayed for matching, and format_game stamps it on every game
	// split_variants emits.
	HashCodeReplay *replay.Result

	// Ratings adds WhiteElo/BlackElo tags from the players' names
	// (--addelotags); IDs adds WhiteFideId/BlackFideId (--addfideidtags).
	// A player missing from the table adds nothing.
	Ratings, IDs PlayerInfo

	// LineNumbers prepends a "MARKER:start:end" comment naming the lines
	// the game occupied in its source (--linenumbers MARKER). "" is off.
	LineNumbers string

	// KeepShort keeps a game too short for DropPlies or DropBefore
	// (--keepbroken): the drop is skipped and the other transforms run,
	// and Apply still returns the *ShortGameError so the caller can
	// report it. Without it such a game is left untouched and the error
	// alone is returned.
	KeepShort bool
}

// enabled reports whether opts asks for any transform at all.
func (opts Options) enabled() bool {
	return opts.DropPlies != 0 || opts.DropBefore != "" || opts.PlyCount || opts.TotalPlyCount ||
		opts.HashCode || opts.Ratings != nil || opts.IDs != nil || opts.LineNumbers != ""
}

// ShortGameError reports a game with fewer plies than --dropply asked to
// drop (or, for a negative count, to keep). The C tool's rewrite fails
// on such a game: without --keepbroken it is emitted nowhere, with it the
// game is emitted untouched.
type ShortGameError struct {
	// Plies is the number asked for, Have the number the game has.
	Plies, Have int
	// Keep is true when Plies was to be kept rather than dropped.
	Keep bool
}

func (e *ShortGameError) Error() string {
	if e.Keep {
		return fmt.Sprintf("cannot keep the last %d plies of a game of %d", e.Plies, e.Have)
	}
	return fmt.Sprintf("cannot drop %d plies from a game of %d", e.Plies, e.Have)
}

// Apply rewrites g in place according to opts, in pgn-extract's
// format_game order: the line-number comment, ply dropping, the player
// tags, PlyCount, TotalPlyCount, HashCode. r is g's replay, complete or,
// for a broken game, partial; plies are only dropped from a game whose
// replay is complete, as the C tool's rewrite fails before it reaches the
// drop. Apply returns the replay of the game it leaves behind: r itself,
// or a fresh replay from the new FEN when plies were dropped.
//
// Dropping more plies than the game has (or keeping more than it has) is
// a *ShortGameError, returned before anything is changed, so the game
// can be emitted nowhere, as the C tool emits nothing for it. With
// opts.KeepShort the game is instead rewritten as the C tool rewrites
// it under --keepbroken (the drop skipped; the prefix comments, which
// go before the drop is attempted, gone when plies were to be dropped)
// and the error is returned alongside the valid replay for reporting.
func Apply(g *pgn.Game, r *replay.Result, opts Options) (*replay.Result, error) {
	_, hasHashCode := g.Tag("HashCode")
	if !opts.enabled() && !hasHashCode {
		return r, nil
	}

	complete := r != nil && len(r.Moves) == len(g.Moves)
	drop, n, shortErr := pliesToDrop(g, r, opts)
	if shortErr != nil && !opts.KeepShort {
		return nil, shortErr
	}
	drop = drop && complete

	if opts.LineNumbers != "" {
		comment := fmt.Sprintf("%s:%d:%d", opts.LineNumbers, g.StartLine, g.EndLine)
		g.PrefixComments = append([]string{comment}, g.PrefixComments...)
	}

	// The HashCode is the C tool's cumulative_hash_value, accumulated
	// while the full game was replayed for matching, so it is taken
	// before any plies go.
	var hashCode string
	if opts.HashCode || hasHashCode {
		hr := r
		if opts.HashCodeReplay != nil {
			hr = opts.HashCodeReplay
		}
		hashCode = fmt.Sprintf("%08x", uint32(cumulativeHash(hr)))
	}

	if drop {
		var err error
		if r, err = dropPlies(g, r, n, opts); err != nil {
			return nil, err
		}
	} else if shortErr != nil && !shortErr.Keep {
		// drop_plies_from_start lets the prefix comments go before it
		// finds the game short.
		g.PrefixComments = nil
	}

	if opts.Ratings != nil {
		addPlayerTag(g, "White", "WhiteElo", opts.Ratings)
		addPlayerTag(g, "Black", "BlackElo", opts.Ratings)
	}
	if opts.IDs != nil {
		addPlayerTag(g, "White", "WhiteFideId", opts.IDs)
		addPlayerTag(g, "Black", "BlackFideId", opts.IDs)
	}
	if opts.PlyCount {
		g.SetTag("PlyCount", strconv.Itoa(len(g.Moves)))
	}
	if opts.TotalPlyCount {
		g.SetTag("TotalPlyCount", strconv.Itoa(countPlies(g.Moves, opts.CountVariations)))
	}
	if hashCode != "" {
		g.SetTag("HashCode", hashCode)
	}
	if shortErr != nil {
		return r, shortErr
	}
	return r, nil
}

// cumulativeHash is apply.c's cumulative_hash_value: the wrapping sum of
// the weak hash of the position after each main-line move of r.
func cumulativeHash(r *replay.Result) uint64 {
	if r == nil {
		return 0
	}
	b := r.StartBoard()
	var sum uint64
	for _, m := range r.Moves {
		b.Apply(m.Move)
		sum += b.WeakHash()
	}
	return sum
}

// countPlies is output.c count_move_list_ply.
func countPlies(moves []*pgn.Move, variations bool) int {
	n := len(moves)
	if !variations {
		return n
	}
	for _, m := range moves {
		for _, v := range m.Variations() {
			n += countPlies(v.Moves, true)
		}
	}
	return n
}

// pliesToDrop is the tail of apply.c rewrite_moves: it resolves whether
// plies are to be dropped and how many (a --dropbefore match wins over
// --dropply; a negative count keeps that many; a resolved count of zero
// still pins the start as a FEN), or returns a *ShortGameError when the
// game is too short. r may be partial; the count is resolved regardless,
// as the C tool resolves it before the rewrite can fail.
func pliesToDrop(g *pgn.Game, r *replay.Result, opts Options) (drop bool, n int, err *ShortGameError) {
	n = opts.DropPlies
	if opts.DropBefore != "" && r != nil {
		for i, m := range g.Moves {
			if i+1 >= len(r.Positions) {
				break
			}
			// Only the comments written directly after the move count
			// (find_matching_comment reads comment_list): one that
			// follows a NAG or a variation does not drop anything.
			if commentMatches(m.LeadingComments(), opts.DropBefore) {
				// The C tool counts from the board's move number, not
				// from the game's first move, so for a FEN start this is
				// the absolute ply count (Position.PlyNumber), which can
				// exceed the plies the game actually has.
				n = r.Positions[i+1].PlyNumber() - 1
			}
		}
	}
	if n == 0 {
		return false, 0, nil
	}
	if n < 0 {
		if -n > len(g.Moves) {
			return false, 0, &ShortGameError{Plies: -n, Have: len(g.Moves), Keep: true}
		}
		n += len(g.Moves)
	}
	if n > len(g.Moves) {
		return false, 0, &ShortGameError{Plies: n, Have: len(g.Moves)}
	}
	return true, n, nil
}

// dropPlies is apply.c drop_plies_from_start: it drops the first n
// plies, pins the new start as FEN and SetUp tags, and recounts PlyCount
// and TotalPlyCount when the game carries them or they were asked for.
// The prefix comments go with the dropped plies, but stay when n is 0.
func dropPlies(g *pgn.Game, r *replay.Result, n int, opts Options) (*replay.Result, error) {
	if n > 0 {
		g.PrefixComments = nil
	}
	g.Moves = g.Moves[n:]
	g.SetTag("FEN", r.BoardAt(n).FEN())
	g.SetTag("SetUp", "1")
	if _, ok := g.Tag("PlyCount"); ok || opts.PlyCount {
		g.SetTag("PlyCount", strconv.Itoa(len(g.Moves)))
	}
	if _, ok := g.Tag("TotalPlyCount"); ok || opts.TotalPlyCount {
		g.SetTag("TotalPlyCount", strconv.Itoa(countPlies(g.Moves, opts.CountVariations)))
	}
	// The run's null-move policy has already judged the game: a
	// main-line null move either passed under --allow-null-moves or made
	// the game Broken, and under --keep-broken a Broken game still goes
	// out (rewrite_moves keeps the board). Either way the remainder is
	// replayed as it stands, so that the null move is not refused a
	// second time here and the dropped game silently lost.
	nr, err := replay.Game(g, replay.Options{AllowNullMoves: true})
	if err != nil {
		return nil, fmt.Errorf("replaying from the dropped-ply position: %w", err)
	}
	return nr, nil
}

// commentMatches is apply.c find_matching_comment: an exact comparison of
// the pattern with each comment's text, which the C lexer stores without
// its surrounding whitespace.
func commentMatches(comments []string, pattern string) bool {
	for _, c := range comments {
		if strings.TrimSpace(c) == pattern {
			return true
		}
	}
	return false
}

// addPlayerTag sets tagName from info when the player named by playerTag
// is in it (output.c add_player_tag).
func addPlayerTag(g *pgn.Game, playerTag, tagName string, info PlayerInfo) {
	name, ok := g.Tag(playerTag)
	if !ok || name == "" {
		return
	}
	if value, ok := info[name]; ok {
		g.SetTag(tagName, value)
	}
}
