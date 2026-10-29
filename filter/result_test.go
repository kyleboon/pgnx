package filter

import (
	"errors"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
)

func TestNormaliseResult(t *testing.T) {
	cases := []struct {
		tags  []string
		token string
		want  string
		has   bool
	}{
		{[]string{"Result", "1/2"}, "1/2-1/2", "1/2-1/2", true},
		{[]string{"Result", "1/2"}, "", "1/2-1/2", true},
		{[]string{"Result", "?"}, "1-0", "1-0", true},
		{[]string{"Result", ""}, "0-1", "0-1", true},
		{nil, "1-0", "1-0", true},
		{nil, "", "", false},
		{[]string{"Result", "?"}, "", "?", true},
		{[]string{"Result", "1-0"}, "0-1", "1-0", true}, // consistency is checked later, not here
	}
	for _, c := range cases {
		g := game(c.tags...)
		g.Result = c.token
		normaliseResult(g)
		got, has := g.Tag("Result")
		if got != c.want || has != c.has {
			t.Errorf("%v token %q: Result = %q,%v want %q,%v", c.tags, c.token, got, has, c.want, c.has)
		}
	}
}

func TestConsistentFENTags(t *testing.T) {
	const fen = "4k3/8/8/8/8/8/8/4K2R w K - 0 1"
	g := game("FEN", fen)
	if !consistentFENTags(g) {
		t.Error("valid FEN is consistent")
	}
	if v, ok := g.Tag("SetUp"); !ok || v != "1" {
		t.Errorf("SetUp added: %q %v", v, ok)
	}
	g = game("SetUp", "1", "FEN", fen)
	consistentFENTags(g)
	if n := len(g.Tags); n != 2 {
		t.Errorf("no duplicate SetUp: %v", g.Tags)
	}
	if consistentFENTags(game("SetUp", "1")) {
		t.Error("SetUp without FEN is inconsistent")
	}
	if !consistentFENTags(game("SetUp", "0")) {
		t.Error("SetUp 0 without FEN is fine")
	}
	if consistentFENTags(game("FEN", "garbage")) {
		t.Error("bad FEN is inconsistent")
	}
	if !consistentFENTags(game("White", "x")) {
		t.Error("no FEN tags")
	}
}

func TestCheckResultFix(t *testing.T) {
	cases := []struct {
		name      string
		src       string
		wantTag   string
		wantToken string
	}{
		{"mate by black with 1-0 tag", "[Result \"1-0\"]\n\n" + foolsMate, "0-1", "0-1"},
		{"mate by white with * tag", "[Result \"*\"]\n[FEN \"7k/8/6K1/8/8/8/8/Q7 w - - 0 1\"]\n\n1. Qa8# *\n", "1-0", "1-0"},
		{"stalemate with 1-0 tag", "[Result \"1-0\"]\n[FEN \"7k/8/6Q1/8/8/8/8/K7 w - - 0 1\"]\n\n1. Qf7 1-0\n", "1/2-1/2", "1/2-1/2"},
		{"insufficient with 0-1 tag", "[Result \"0-1\"]\n[FEN \"7k/8/8/8/8/8/1r6/K7 w - - 0 1\"]\n\n1. Kxb2 0-1\n", "1/2-1/2", "1/2-1/2"},
		{"tag * vs token 1-0", "[Result \"*\"]\n\n1. e4 1-0\n", "1-0", "1-0"},
		{"tag 1-0 vs token *", "[Result \"1-0\"]\n\n1. e4 *\n", "*", "*"},
		{"irreconcilable", "[Result \"1-0\"]\n\n1. e4 0-1\n", "1-0", "0-1"},
		{"consistent", "[Result \"1-0\"]\n\n1. e4 1-0\n", "1-0", "1-0"},
		{"draw tag with stalemate", "[Result \"1/2-1/2\"]\n[FEN \"7k/8/6Q1/8/8/8/8/K7 w - - 0 1\"]\n\n1. Qf7 1/2-1/2\n", "1/2-1/2", "1/2-1/2"},
	}
	for _, c := range cases {
		g, r := replayed(t, c.src)
		if err := checkResult(g, r, true, false); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		tag, _ := g.Tag("Result")
		if tag != c.wantTag || g.Result != c.wantToken {
			t.Errorf("%s: tag %q token %q, want %q %q", c.name, tag, g.Result, c.wantTag, c.wantToken)
		}
	}
}

func TestCheckResultReject(t *testing.T) {
	cases := []struct {
		src    string
		reject bool
	}{
		{"[Result \"1-0\"]\n\n1. e4 *\n", false},
		{"[Result \"0-1\"]\n\n1. e4 *\n", false},
		{"[Result \"1-0\"]\n\n1. e4 0-1\n", true},
		{"[Result \"0-1\"]\n\n1. e4 1/2-1/2\n", true},
		{"[Result \"*\"]\n\n1. e4 1-0\n", true},
		{"[Result \"1-0\"]\n\n1. e4 1-0\n", false},
		{"[Result \"1-0\"]\n\n" + foolsMate, true},               // tag 1-0 vs token 0-1
		{"[Result \"0-1\"]\n\n1. f3 e5 2. g4 Qh4# 1-0\n", true},  // mate agrees with tag, token disagrees
		{"[Result \"1-0\"]\n\n1. f3 e5 2. g4 Qh4# 1-0\n", false}, // mate disagrees with both: warn only
		{"1. e4 0-1\n", false},                                   // no tag
		{"[Result \"1-0\"]\n\n*\n", false},                       // no moves
	}
	for _, c := range cases {
		g, r := replayed(t, c.src)
		err := checkResult(g, r, false, true)
		if got := errors.Is(err, ErrBadResult); got != c.reject {
			t.Errorf("%q: reject = %v (err %v), want %v", c.src, got, err, c.reject)
		}
		if tag, _ := g.Tag("Result"); err == nil && c.src[0] == '[' && tag == "" {
			t.Errorf("%q: tag should be untouched without fix", c.src)
		}
	}
}

func TestEnsureValidResultTag(t *testing.T) {
	g := game("Result", "1:0")
	ensureValidResultTag(g)
	if v, _ := g.Tag("Result"); v != "*" {
		t.Errorf("invalid result → *, got %q", v)
	}
	g = game("Result", "1/2-1/2")
	ensureValidResultTag(g)
	if v, _ := g.Tag("Result"); v != "1/2-1/2" {
		t.Errorf("valid result kept, got %q", v)
	}
	g = &pgn.Game{}
	ensureValidResultTag(g)
	if _, ok := g.Tag("Result"); ok {
		t.Error("absent stays absent")
	}
}
