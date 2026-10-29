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

func TestParseCriterionPositional(t *testing.T) {
	cases := []struct {
		line       string
		tag, value string
		label      string
	}{
		{`FEN "8/8/8/8/8/8/8/K6k w - - 0 1"`, "FEN", "8/8/8/8/8/8/8/K6k w - - 0 1", ""},
		{`FEN "8/8/8/8/8/8/8/K6k w - - 0 1" ignored`, "FEN", "8/8/8/8/8/8/8/K6k w - - 0 1", ""},
		{`FENPattern "*/*/*/*/???N????/*/*/*" Nd4th`, "FENPattern", "*/*/*/*/???N????/*/*/*", "Nd4th"},
		{`FENPatternI "*/*/*/*/???N????/*/*/*" Nd4th`, "FENPatternI", "*/*/*/*/???N????/*/*/*", "Nd4th"},
		{`FENPatternI "*/*/*/*/???N????/*/*/*"   Nd4 th  `, "FENPatternI", "*/*/*/*/???N????/*/*/*", "Nd4 th"},
		{`FENPattern "*/*/*/*/???N????/*/*/*"   `, "FENPattern", "*/*/*/*/???N????/*/*/*", ""},
		{`FENPattern "*/*/*/*/???N????/*/*/*"`, "FENPattern", "*/*/*/*/???N????/*/*/*", ""},
		{"FENPattern \"*/*/*/*/???N????/*/*/*\" lab\r", "FENPattern", "*/*/*/*/???N????/*/*/*", "lab"},
	}
	for _, c := range cases {
		got, err := ParseCriterion(c.line)
		if err != nil {
			t.Errorf("%q: %v", c.line, err)
			continue
		}
		if got.Tag != c.tag || got.Value != c.value || got.Label != c.label {
			t.Errorf("%q: got %+v, want tag %q value %q label %q", c.line, got, c.tag, c.value, c.label)
		}
		if !got.IsPositional() {
			t.Errorf("%q: IsPositional false", c.line)
		}
	}
	// Non-positional criteria never carry a label and are not positional.
	got, err := ParseCriterion(`White "Fischer" trailing`)
	if err != nil || got.Label != "" || got.IsPositional() {
		t.Errorf("White criterion: %+v err %v", got, err)
	}
	// A missing value is still an error for the positional names.
	if _, err := ParseCriterion(`FENPattern Nd4th`); err == nil {
		t.Error("FENPattern without a quoted value: want error")
	}
}

func TestTagMatcherRejectsPositional(t *testing.T) {
	for _, line := range []string{
		`FEN "8/8/8/8/8/8/8/K6k w - - 0 1"`,
		`FENPattern "*/*/*/*/???N????/*/*/*" L`,
		`FENPatternI "*/*/*/*/???N????/*/*/*"`,
	} {
		c, err := ParseCriterion(line)
		if err != nil {
			t.Fatalf("%q: %v", line, err)
		}
		m := &TagMatcher{}
		if err := m.Add(c); !errors.Is(err, ErrPositionalCriterion) {
			t.Errorf("%q: Add returned %v, want ErrPositionalCriterion", line, err)
		}
		if !m.Empty() {
			t.Errorf("%q: criterion was added despite the error", line)
		}
	}
	m := &TagMatcher{}
	if err := m.Add(Criterion{Tag: "White", Value: "Fischer"}); err != nil {
		t.Errorf("ordinary criterion: %v", err)
	}
}

func TestReadCriteriaKeepsPositionalLines(t *testing.T) {
	src := "White \"Fischer\"\nFENPattern \"*/*/*/*/???N????/*/*/*\" Nd4th\nFEN \"8/8/8/8/8/8/8/K6k w - - 0 1\"\n"
	cs, err := ReadCriteria(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 || cs[0].IsPositional() || !cs[1].IsPositional() || cs[1].Label != "Nd4th" || !cs[2].IsPositional() {
		t.Errorf("criteria: %+v", cs)
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
