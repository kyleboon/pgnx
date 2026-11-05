package dedup

import "github.com/kyleboon/pgnx/chess"

// SetupSet is the set of starting positions a run has met
// (hashing.c check_duplicate_setup, behind --deletesamesetup). It
// satisfies filter.SetupSeen. The zero value is an empty set.
type SetupSet struct {
	seen          map[uint64]struct{}
	standardStart bool
}

// Seen reports whether the starting position fen describes has been
// offered before, registering it if not. "" is the standard start, kept
// as a flag so games without a FEN tag never hash (standard_start_seen);
// any other FEN is identified by its full Polyglot hash
// (generate_zobrist_hash_from_fen), so castling rights, en passant and
// side to move count and the clocks do not. A FEN that does not parse is
// never seen and never registered: pipeline.Normalise rejects such a
// game before the criteria run.
func (s *SetupSet) Seen(fen string) bool {
	if fen == "" {
		seen := s.standardStart
		s.standardStart = true
		return seen
	}
	b, err := chess.FromFEN(fen)
	if err != nil {
		return false
	}
	h := b.Zobrist()
	if _, ok := s.seen[h]; ok {
		return true
	}
	if s.seen == nil {
		s.seen = make(map[uint64]struct{})
	}
	s.seen[h] = struct{}{}
	return false
}
