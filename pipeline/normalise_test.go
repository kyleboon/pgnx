package pipeline

import (
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
)

// game builds a parsed game from name/value tag pairs and a result token.
func game(token string, tags ...string) *pgn.Game {
	g := &pgn.Game{Result: token}
	for i := 0; i+1 < len(tags); i += 2 {
		g.Tags = append(g.Tags, pgn.Tag{Name: tags[i], Value: tags[i+1]})
	}
	return g
}

// Oracle: grammar.c check_result. "1/2" is rewritten to "1/2-1/2"; when the
// movetext ends with a result token, an absent, empty or "?" tag is filled
// from it. Anything else is left for the criteria stage.
func TestNormaliseResultTag(t *testing.T) {
	cases := []struct {
		name  string
		tags  []string
		token string
		want  string
		has   bool
	}{
		{"short draw with matching token", []string{"Result", "1/2"}, "1/2-1/2", "1/2-1/2", true},
		{"short draw with no token", []string{"Result", "1/2"}, "", "1/2-1/2", true},
		{"short draw with other token", []string{"Result", "1/2"}, "*", "1/2-1/2", true},
		{"? filled from token", []string{"Result", "?"}, "1-0", "1-0", true},
		{"empty filled from token", []string{"Result", ""}, "0-1", "0-1", true},
		{"absent filled from token", nil, "1-0", "1-0", true},
		{"absent and no token stays absent", nil, "", "", false},
		{"? and no token stays ?", []string{"Result", "?"}, "", "?", true},
		{"empty and no token stays empty", []string{"Result", ""}, "", "", true},
		{"disagreement is left alone", []string{"Result", "1-0"}, "0-1", "1-0", true},
		{"invalid value is left alone", []string{"Result", "1:0"}, "1-0", "1:0", true},
		{"last of duplicate tags is the one rewritten", []string{"Result", "1-0", "Result", "1/2"}, "", "1/2-1/2", true},
	}
	for _, c := range cases {
		g := game(c.token, c.tags...)
		if err := Normalise(g); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		got, has := g.Tag("Result")
		if got != c.want || has != c.has {
			t.Errorf("%s: Result = %q,%v want %q,%v", c.name, got, has, c.want, c.has)
		}
		if g.Result != c.token {
			t.Errorf("%s: token changed to %q", c.name, g.Result)
		}
	}
}

func TestNormaliseResultDoesNotDuplicateTag(t *testing.T) {
	g := game("1-0", "Event", "e", "Result", "?", "White", "w")
	if err := Normalise(g); err != nil {
		t.Fatal(err)
	}
	want := []pgn.Tag{{Name: "Event", Value: "e"}, {Name: "Result", Value: "1-0"}, {Name: "White", Value: "w"}}
	if len(g.Tags) != len(want) {
		t.Fatalf("tags = %v, want %v", g.Tags, want)
	}
	for i := range want {
		if g.Tags[i] != want[i] {
			t.Errorf("tag %d = %v, want %v", i, g.Tags[i], want[i])
		}
	}
}

const fen = "4k3/8/8/8/8/8/8/4K2R w K - 0 1"

// Oracle: grammar.c consistent_FEN_tags. A valid FEN without SetUp gains
// SetUp "1" (silently); SetUp "1" without a FEN, or an unparsable FEN, is
// inconsistent and the game is rejected.
func TestNormaliseFENTags(t *testing.T) {
	g := game("", "Event", "e", "FEN", fen)
	if err := Normalise(g); err != nil {
		t.Fatal(err)
	}
	if v, ok := g.Tag("SetUp"); !ok || v != "1" {
		t.Errorf("SetUp added: %q %v", v, ok)
	}
	if len(g.Tags) != 3 || g.Tags[2].Name != "SetUp" {
		t.Errorf("SetUp appended once: %v", g.Tags)
	}

	g = game("", "SetUp", "1", "FEN", fen)
	if err := Normalise(g); err != nil {
		t.Fatal(err)
	}
	if len(g.Tags) != 2 {
		t.Errorf("consistent tags untouched: %v", g.Tags)
	}

	if err := Normalise(game("", "SetUp", "0")); err != nil {
		t.Errorf("SetUp 0 without FEN is fine: %v", err)
	}
	if err := Normalise(game("", "White", "x")); err != nil {
		t.Errorf("no FEN tags: %v", err)
	}
}

func TestNormaliseRejectsInconsistentFENTags(t *testing.T) {
	err := Normalise(game("", "SetUp", "1"))
	if err == nil {
		t.Fatal("SetUp 1 without FEN must be rejected")
	}
	if !strings.Contains(err.Error(), "SetUp") || !strings.Contains(err.Error(), "FEN") {
		t.Errorf("reason names both tags: %v", err)
	}

	g := game("", "FEN", "garbage")
	err = Normalise(g)
	if err == nil {
		t.Fatal("unparsable FEN must be rejected")
	}
	if !strings.Contains(err.Error(), `"garbage"`) {
		t.Errorf("reason quotes the FEN: %v", err)
	}
	if _, ok := g.Tag("SetUp"); ok {
		t.Error("no SetUp added to a rejected game")
	}
}
