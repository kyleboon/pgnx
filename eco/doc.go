// Package eco implements pgn-extract's ECO classification (-e FILE):
// building a table of opening lines from a PGN file of ECO entries,
// classifying a replayed game against that table, and replacing the
// game's ECO, Opening, Variation and SubVariation tags with the matched
// line's.
//
// The table mirrors eco.c (save_eco_details, eco_matches) and the
// classification walk mirrors the per-ply block in apply.c play_moves:
// every main-line position after a move is looked up by its placement
// key, an exact match (same ply count, side to move and cumulative
// placement key) beats a partial one within a six-ply window, the
// earliest-loaded partial candidate wins, and the match found at the
// latest ply replaces any earlier one. Variations are never classified
// and the start position is never tested.
//
// Classification is a tag-adding transform rather than a selection
// criterion, so it lives apart from the filter package; the CLI runs
// Classify and Apply before filter.Evaluate so that ECO tag criteria see
// the classified tags. eco imports chess, pgn and replay and never writes
// output.
package eco
