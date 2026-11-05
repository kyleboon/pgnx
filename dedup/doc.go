// Package dedup implements pgn-extract's duplicate detection (hashing.c):
// the identity of a game for the purpose, the seen-set a run registers
// games in, and the set of starting positions behind --deletesamesetup.
//
// A game's identity (Key, KeyOf) is the placement key of its final
// main-line position together with the wrapping sum of the placement
// keys after each main-line move, which is how apply.c play_moves
// accumulates the C tool's final_hash_value and cumulative_hash_value;
// variations never contribute. pgnx uses replay.Position.Placement
// rather than the C tool's weak hash: both are placement-only keys whose
// only observable property here is equality. Fuzzy identity (FuzzyKey,
// Options.FuzzyDepth) is the placement after the move with a given
// absolute ply number, or the final placement at depth 0.
//
// Log is the seen-set (previous_occurance): Visit reports whether an
// equivalent game was registered before and from which source, and
// registers the game when none was. SetupSet is check_duplicate_setup's
// table of starting positions, offered to filter.Evaluate through its
// SetupSeen hook.
//
// dedup never mutates a game and writes nothing; the command that owns
// a Log decides what a duplicate means for routing. Log and SetupSet are
// not safe for concurrent use: a run visits its games in order. dedup
// imports chess and replay only.
package dedup
