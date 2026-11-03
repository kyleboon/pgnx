package pgn

import (
	"strconv"
	"strings"
	"testing"
)

// parseOne parses src as a single game.
func parseOne(t *testing.T, src string) *Game {
	t.Helper()
	s := NewScanner(strings.NewReader(src), ParserOptions{})
	if !s.Next() {
		t.Fatalf("no game parsed: %v", s.Err())
	}
	return s.Game()
}

// describe renders a move's annotations as a compact string for
// assertions: "$N" for a NAG, "{text}" for a comment, "(san ...)" for a
// variation (its moves' SAN, recursively described).
func describe(m *Move) string {
	var parts []string
	for _, a := range m.Annotations {
		switch a := a.(type) {
		case NAG:
			parts = append(parts, "$"+strconv.Itoa(int(a)))
		case Comment:
			parts = append(parts, "{"+string(a)+"}")
		case *Variation:
			var inner []string
			for _, c := range a.PrefixComments {
				inner = append(inner, "{"+c+"}")
			}
			for _, vm := range a.Moves {
				inner = append(inner, vm.SAN)
				if d := describe(vm); d != "" {
					inner = append(inner, d)
				}
			}
			if a.Result != "" {
				inner = append(inner, a.Result)
			}
			parts = append(parts, "("+strings.Join(inner, " ")+")")
		}
	}
	return strings.Join(parts, " ")
}

func TestMoveAnnotationsKeepSourceOrder(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string // describe() of the first move
	}{
		{"nag comment interleaving",
			`1. b3 { good or bad? } $1 { some think this is good } $2 { while others think it is not } *`,
			"{ good or bad? } $1 { some think this is good } $2 { while others think it is not }"},
		{"comment after a variation trails the move before it",
			`1. e4 { a } (1. d4) { s } (1. c4) { u } e5 *`,
			"{ a } (d4) { s } (c4) { u }"},
		{"nag after a variation is kept in place",
			`1. e4 (1. d4) $9 { b } e5 *`,
			"(d4) $9 { b }"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			g := parseOne(t, c.src)
			if got := describe(g.Moves[0]); got != c.want {
				t.Errorf("annotations of %q:\n got %s\nwant %s", g.Moves[0].SAN, got, c.want)
			}
		})
	}
}

func TestVariationKeepsItsResultAndTrailingComment(t *testing.T) {
	g := parseOne(t, `1. e4 e5 (1... c5 $1 { x } 1-0 { y }) { z } 2. Nf3 *`)
	// The comment after the variation's result stays where it was written
	// (source fidelity; the C tool moves it before the NAG, see
	// CONFORMANCE.md "Ordered annotations").
	if got, want := describe(g.Moves[1]), "(c5 $1 { x } { y } 1-0) { z }"; got != want {
		t.Errorf("annotations of e5:\n got %s\nwant %s", got, want)
	}
	if len(g.ParseErrors()) != 0 {
		t.Errorf("unexpected parse errors: %v", g.ParseErrors())
	}
	if g.Result != "*" {
		t.Errorf("game result = %q, want *", g.Result)
	}
}

func TestAddCommentGoesAfterLeadingComments(t *testing.T) {
	g := parseOne(t, `1. e4 { a } $1 { b } (1. d4) { s } e5 *`)
	g.Moves[0].AddComment("mark")
	if got, want := describe(g.Moves[0]), "{ a } {mark} $1 { b } (d4) { s }"; got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	g.Moves[1].AddComment("mark")
	if got, want := describe(g.Moves[1]), "{mark}"; got != want {
		t.Errorf("got %s\nwant %s", got, want)
	}
	if got := g.Moves[0].LeadingComments(); len(got) != 2 || got[1] != "mark" {
		t.Errorf("LeadingComments = %q", got)
	}
	if got := g.Moves[0].Comments(); len(got) != 4 {
		t.Errorf("Comments = %q", got)
	}
}
