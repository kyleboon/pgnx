// Package pipeline owns pgn-extract's per-game semantics: what happens to
// a parsed game between the parser and the writer, in the order the C
// tool's deal_with_game (grammar.c) does it. The pgn package preserves
// the source; this package corrects and judges it. See ADR 0002.
//
// The stages are normalisation (Normalise), replay, disposition against
// keep-broken, ECO classification and criteria evaluation. Every command
// is a caller; commands differ only in what they do with the outcome.
// Each stage touches only its game and read-only configuration, so the
// stages are safe to run concurrently over different games; everything
// order-sensitive (tag-registry observation, selection, routing, writing)
// stays with the caller.
package pipeline
