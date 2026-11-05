package dedup

import "github.com/kyleboon/pgnx/replay"

// Options configures a Log's notion of identity.
type Options struct {
	// Fuzzy enables positional identity (--fuzzydepth): besides the
	// exact Key, two games are duplicates when they share the position
	// FuzzyKey selects at FuzzyDepth (the final position at depth 0),
	// whatever moves reached it.
	Fuzzy      bool
	FuzzyDepth int
}

// Log is the seen-set of a run (hashing.c LogTable and
// previous_occurance): every game registered so far, by Key, with the
// source it was first seen in. The zero Log is not usable; use NewLog.
type Log struct {
	opts Options
	// exact holds the stored pair of every registered game: its Key, or
	// under fuzzy identity (FuzzyDepth > 0, game long enough) the pair
	// (fuzzy key, 0), as the C tool stores it. fuzzy indexes the same
	// entries by their stored Final alone, for the fuzzy pass; nil
	// unless opts.Fuzzy. Values index sources.
	exact map[Key]uint32
	fuzzy map[uint64]uint32
	// sources are the distinct source names games were registered from;
	// sourceIndex maps a name to its index.
	sources     []string
	sourceIndex map[string]uint32
	n           int
}

// NewLog returns an empty seen-set with the given identity.
func NewLog(o Options) *Log {
	l := &Log{opts: o, exact: make(map[Key]uint32), sourceIndex: make(map[string]uint32)}
	if o.Fuzzy {
		l.fuzzy = make(map[uint64]uint32)
	}
	return l
}

// Visit is previous_occurance: it reports whether an equivalent game has
// been registered before and, if so, the source that game came from;
// otherwise it registers r under source. The exact pass looks the game's
// Key up; under Options.Fuzzy a second pass looks its fuzzy key up
// against the stored Final of every registered game (so a short game's
// final position can be another game's position at the fuzzy depth; the
// first registered game is the origin). On a miss the game is stored as
// (fuzzy key, 0) when fuzzy identity is on, FuzzyDepth > 0 and the game
// has that ply, else as its Key. r may be the partial replay of a broken
// game (apply.c: the hash values are filled in whether or not the moves
// were all legal).
func (l *Log) Visit(r *replay.Result, source string) (origin string, duplicate bool) {
	k := KeyOf(r)
	if idx, ok := l.exact[k]; ok {
		return l.sources[idx], true
	}
	var (
		fk    uint64
		hasFK bool
	)
	if l.opts.Fuzzy {
		fk, hasFK = FuzzyKey(r, l.opts.FuzzyDepth)
		if hasFK {
			if idx, ok := l.fuzzy[fk]; ok {
				return l.sources[idx], true
			}
		}
	}

	stored := k
	if l.opts.Fuzzy && l.opts.FuzzyDepth > 0 && hasFK {
		stored = Key{Final: fk}
	}
	idx := l.source(source)
	if _, ok := l.exact[stored]; !ok {
		l.exact[stored] = idx
	}
	if l.fuzzy != nil {
		if _, ok := l.fuzzy[stored.Final]; !ok {
			l.fuzzy[stored.Final] = idx
		}
	}
	l.n++
	return "", false
}

// Len is the number of games registered: those visited that were not
// duplicates.
func (l *Log) Len() int { return l.n }

// source returns the index of name in sources, adding it if new.
func (l *Log) source(name string) uint32 {
	if idx, ok := l.sourceIndex[name]; ok {
		return idx
	}
	idx := uint32(len(l.sources))
	l.sources = append(l.sources, name)
	l.sourceIndex[name] = idx
	return idx
}
