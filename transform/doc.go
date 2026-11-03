// Package transform rewrites a game on its way to the writer, as
// pgn-extract's format_game (output.c) and rewrite_game (apply.c) do
// after a game has been selected: the line-number comment
// (--linenumbers), ply dropping (--dropply, --dropbefore) with the FEN and
// SetUp tags that make the remainder replayable, and the tags computed
// from the game (--plycount, --totalplycount, --addhashcode) or looked up
// for its players (--addelotags, --addfideidtags). Apply runs them in the
// C tool's order over one game and its replay; the pipeline calls it last,
// after classification and the criteria, so selection always sees the
// game as parsed.
//
// Options that change how the game is rendered rather than what it holds
// (--plylimit, --fencomments, --notags, ...) are output.Options, not
// transforms.
package transform
