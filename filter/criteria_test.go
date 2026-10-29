package filter

import (
	"errors"
	"strings"
	"testing"
)

func TestParseCriterion(t *testing.T) {
	cases := []struct {
		line  string
		want  Criterion
		isErr bool
	}{
		{`White "Fischer"`, Criterion{Tag: "White", Op: OpPrefix, Value: "Fischer"}, false},
		{`WhiteElo >= "800"`, Criterion{Tag: "WhiteElo", Op: OpGE, Value: "800"}, false},
		{`BlackTitle <> "BOT"`, Criterion{Tag: "BlackTitle", Op: OpNE, Value: "BOT"}, false},
		{`Date<"1962"`, Criterion{Tag: "Date", Op: OpLT, Value: "1962"}, false},
		{`Date <= "1962"`, Criterion{Tag: "Date", Op: OpLE, Value: "1962"}, false},
		{`WhiteElo > "2500"`, Criterion{Tag: "WhiteElo", Op: OpGT, Value: "2500"}, false},
		{`Round = "5"`, Criterion{Tag: "Round", Op: OpEQ, Value: "5"}, false},
		{`Event =~ "^World.*"`, Criterion{Tag: "Event", Op: OpRegex, Value: "^World.*"}, false},
		{"  TimeControl >= \"180\"\r", Criterion{Tag: "TimeControl", Op: OpGE, Value: "180"}, false},
		{`Difficulty "5"`, Criterion{Tag: "Difficulty", Op: OpPrefix, Value: "5"}, false},
		{`Player "Fischer" trailing junk`, Criterion{Tag: "Player", Op: OpPrefix, Value: "Fischer"}, false},
		{`Event "a\"b\\c\nd"`, Criterion{Tag: "Event", Op: OpPrefix, Value: `a"b\c\nd`}, false},
		{`Event ""`, Criterion{Tag: "Event", Op: OpPrefix, Value: ""}, false},
		{`[Event "x"]`, Criterion{}, true},
		{`White Fischer`, Criterion{}, true},
		{`White "Fischer`, Criterion{}, true},
		{`White "Fischer\`, Criterion{}, true},
		{`Event =~ "("`, Criterion{}, true},
		{``, Criterion{}, true},
		{`FEN "rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1"`, Criterion{}, true},
		{`FENPattern "..."`, Criterion{}, true},
	}
	for _, c := range cases {
		got, err := ParseCriterion(c.line)
		if c.isErr {
			if err == nil {
				t.Errorf("%q: want error, got %+v", c.line, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("%q: %v", c.line, err)
			continue
		}
		if got.Tag != c.want.Tag || got.Op != c.want.Op || got.Value != c.want.Value {
			t.Errorf("%q: got %+v, want %+v", c.line, got, c.want)
		}
		if got.Op == OpRegex && got.re == nil {
			t.Errorf("%q: regex not compiled", c.line)
		}
	}
}

func TestParseCriterionUnsupportedFEN(t *testing.T) {
	_, err := ParseCriterion(`FEN "8/8/8/8/8/8/8/K6k w - - 0 1"`)
	if !errors.Is(err, ErrUnsupportedCriterion) {
		t.Errorf("want ErrUnsupportedCriterion, got %v", err)
	}
}

func TestReadCriteria(t *testing.T) {
	src := "% a comment\nWhite \"Fischer\"\r\n\n   \nBlack <> \"Spassky\"\n%Date \"1970\"\n"
	cs, err := ReadCriteria(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 || cs[0].Tag != "White" || cs[0].Value != "Fischer" || cs[1].Op != OpNE {
		t.Errorf("criteria: %+v", cs)
	}
}

func TestReadCriteriaErrorNamesLine(t *testing.T) {
	src := "White \"Fischer\"\n[Event \"x\"]\n"
	_, err := ReadCriteria(strings.NewReader(src))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("want error naming line 2, got %v", err)
	}
}

func TestOpString(t *testing.T) {
	for op, want := range map[Op]string{OpPrefix: "", OpLT: "<", OpLE: "<=", OpNE: "<>", OpGT: ">", OpGE: ">=", OpEQ: "=", OpRegex: "=~"} {
		if got := op.String(); got != want {
			t.Errorf("%d: %q want %q", op, got, want)
		}
	}
}
