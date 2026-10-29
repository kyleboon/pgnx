package filter

import (
	"strings"
	"testing"

	"github.com/kyleboon/pgnx/pgn"
)

// game builds a pgn.Game with the given tag name/value pairs.
func game(kv ...string) *pgn.Game {
	g := &pgn.Game{}
	for i := 0; i+1 < len(kv); i += 2 {
		g.Tags = append(g.Tags, pgn.Tag{Name: kv[i], Value: kv[i+1]})
	}
	return g
}

func matcher(t *testing.T, lines ...string) *TagMatcher {
	t.Helper()
	m := &TagMatcher{}
	for _, l := range lines {
		c, err := ParseCriterion(l)
		if err != nil {
			t.Fatalf("%q: %v", l, err)
		}
		m.Add(c)
	}
	return m
}

func TestTagMatcherPrefixAndSubstring(t *testing.T) {
	g := game("White", "Fischer, Robert J.", "Black", "Spassky, Boris")
	m := matcher(t, `White "Fischer"`)
	if !m.Match(g) {
		t.Error("prefix should match")
	}
	m = matcher(t, `White "Robert"`)
	if m.Match(g) {
		t.Error("non-prefix should not match")
	}
	m.Substring = true
	if !m.Match(g) {
		t.Error("substring should match")
	}
	if (&TagMatcher{}).Match(g) != true || !(&TagMatcher{}).Empty() {
		t.Error("empty matcher matches everything")
	}
}

func TestTagMatcherSameTagORDifferentTagAND(t *testing.T) {
	g := game("White", "Fischer", "Black", "Petrosian", "Date", "1971.10.07")
	if !matcher(t, `Date "1960"`, `Date "1971"`).Match(g) {
		t.Error("same tag should OR")
	}
	if matcher(t, `White "Fischer"`, `Black "Spassky"`).Match(g) {
		t.Error("different tags should AND")
	}
	if !matcher(t, `White "Fischer"`, `Black "Petrosian"`, `Date "1971"`).Match(g) {
		t.Error("all satisfied")
	}
}

func TestTagMatcherMissingTag(t *testing.T) {
	g := game("White", "Fischer")
	if matcher(t, `Annotator "Barnes"`).Match(g) {
		t.Error("missing tag fails")
	}
	if !matcher(t, `Annotator <> "Barnes"`).Match(g) {
		t.Error("missing tag passes when all criteria are <>")
	}
	if matcher(t, `Annotator <> "Barnes"`, `Annotator "X"`).Match(g) {
		t.Error("missing tag fails when a non-<> criterion exists")
	}
}

func TestTagMatcherResultPrefix(t *testing.T) {
	m := matcher(t, `Result "1"`)
	if !m.Match(game("Result", "1-0")) || !m.Match(game("Result", "1/2-1/2")) || m.Match(game("Result", "0-1")) {
		t.Error("-Tr1 matches 1-0 and 1/2-1/2 only")
	}
}

func TestTagMatcherPlayer(t *testing.T) {
	m := &TagMatcher{}
	m.AddPrefix("Player", "Petrosian")
	if !m.Match(game("White", "Petrosian", "Black", "Fischer")) {
		t.Error("white")
	}
	if !m.Match(game("White", "Fischer", "Black", "Petrosian")) {
		t.Error("black")
	}
	if m.Match(game("White", "Fischer", "Black", "Spassky")) {
		t.Error("neither")
	}
	if !m.Match(game("Black", "Petrosian")) {
		t.Error("only black tag")
	}
	if m.Match(game("Event", "x")) {
		t.Error("no player tags")
	}
}

func TestTagMatcherEloOverridesPlayer(t *testing.T) {
	// lists.c:933 assigns the Elo result instead of ANDing it, so a failed
	// Player match is overridden by a passing Elo match. Pinned on purpose.
	m := matcher(t, `Player "Nobody"`, `Elo >= "2500"`)
	if !m.Match(game("White", "Fischer", "Black", "Spassky", "WhiteElo", "2785", "BlackElo", "2660")) {
		t.Error("Elo group overrides Player failure (C quirk)")
	}
	if m.Match(game("White", "Fischer", "Black", "Spassky", "WhiteElo", "2400", "BlackElo", "2300")) {
		t.Error("both fail")
	}
}

func TestTagMatcherDate(t *testing.T) {
	cases := []struct {
		crit []string
		date string
		want bool
	}{
		{[]string{`Date "1970"`}, "1970.03.01", true},
		{[]string{`Date "1970"`}, "1971.03.01", false},
		{[]string{`Date "1960"`, `Date "1962"`}, "1962.05.05", true},
		{[]string{`Date "1960"`, `Date "1962"`}, "1961.05.05", false},
		{[]string{`Date "b1962"`}, "1961.12.31", true},
		{[]string{`Date "b1962"`}, "1962.01.01", false},
		{[]string{`Date "a1962"`}, "1962.05.05", true},
		{[]string{`Date "a1962.05.05"`}, "1962.05.05", false},
		{[]string{`Date >= "1960"`, `Date < "1970"`}, "1965.01.01", true},
		{[]string{`Date >= "1960"`, `Date < "1970"`}, "1970.01.01", false},
		{[]string{`Date < "1970"`}, "????.??.??", false},
		{[]string{`Date < "1970"`}, "62.01.01", false}, // two-digit years excluded
		{[]string{`Date "1962"`}, "1962.??.??", true},
		{[]string{`Date >= "1962.06"`}, "1962.??.??", false}, // month defaults to 1
		{[]string{`Date < "x"`}, "1962.01.01", false},
		// Order-dependent mixing: a prefix after a failed relational re-tests.
		{[]string{`Date < "1900"`, `Date "1962"`}, "1962.01.01", true},
		{[]string{`Date "1900"`, `Date < "1970"`}, "1962.01.01", false}, // relational ANDs onto the failed prefix
	}
	for _, c := range cases {
		m := matcher(t, c.crit...)
		if got := m.Match(game("Date", c.date)); got != c.want {
			t.Errorf("%v on %q = %v, want %v", c.crit, c.date, got, c.want)
		}
	}
}

func TestTagMatcherElo(t *testing.T) {
	cases := []struct {
		crit []string
		g    *pgn.Game
		want bool
	}{
		{[]string{`WhiteElo >= "800"`, `WhiteElo <= "899"`}, game("WhiteElo", "850"), true},
		{[]string{`WhiteElo >= "800"`, `WhiteElo <= "899"`}, game("WhiteElo", "900"), false},
		{[]string{`WhiteElo >= "800"`, `WhiteElo "27"`}, game("WhiteElo", "2600"), true}, // prefix ignored once a relational exists
		{[]string{`WhiteElo "27"`}, game("WhiteElo", "2700"), true},
		{[]string{`WhiteElo "27"`}, game("WhiteElo", "2600"), false},
		{[]string{`WhiteElo >= "800"`}, game("WhiteElo", "?"), false},
		{[]string{`WhiteElo >= "800"`}, game("BlackElo", "850"), false},
		{[]string{`Elo >= "2700"`}, game("WhiteElo", "2500", "BlackElo", "2750"), true},
		{[]string{`Elo >= "2700"`}, game("WhiteElo", "2500", "BlackElo", "2650"), false},
		{[]string{`EloDiff <= "50"`}, game("WhiteElo", "2500", "BlackElo", "2540"), true},
		{[]string{`EloDiff <= "50"`}, game("WhiteElo", "2600", "BlackElo", "2540"), false},
		{[]string{`EloDiff <= "50"`}, game("WhiteElo", "2600"), false},
		{[]string{`EloDiff "4"`}, game("WhiteElo", "2600", "BlackElo", "2540"), true}, // plain ignored
	}
	for _, c := range cases {
		m := matcher(t, c.crit...)
		if got := m.Match(c.g); got != c.want {
			t.Errorf("%v on %v = %v, want %v", c.crit, c.g.Tags, got, c.want)
		}
	}
}

func TestTagMatcherTimeControl(t *testing.T) {
	cases := []struct {
		crit []string
		tc   string
		want bool
	}{
		{[]string{`TimeControl >= "180"`}, "180+2", true},
		{[]string{`TimeControl >= "180"`}, "60+1", false},
		{[]string{`TimeControl >= "180"`}, "*300", true},
		{[]string{`TimeControl >= "180"`}, "40/7200:3600", true},
		{[]string{`TimeControl >= "180"`}, "600", true},
		{[]string{`TimeControl >= "180"`}, "-", false},
		{[]string{`TimeControl >= "180"`}, "?", false},
		{[]string{`TimeControl >= "180"`}, "", false},
		{[]string{`TimeControl >= "1000"`, `TimeControl <= "100"`}, "180+2", false},
		{[]string{`TimeControl >= "1000"`, `TimeControl "18"`}, "180+2", true}, // ORed incl. relational
		{[]string{`TimeControl >= "180"`}, "abc", false},
	}
	for _, c := range cases {
		m := matcher(t, c.crit...)
		if got := m.Match(game("TimeControl", c.tc)); got != c.want {
			t.Errorf("%v on %q = %v, want %v", c.crit, c.tc, got, c.want)
		}
	}
}

func TestTagMatcherReltags(t *testing.T) {
	src := "TimeControl >= \"180\"\nWhiteElo >= \"800\"\nWhiteElo <= \"899\"\nBlackElo >= \"800\"\nBlackElo <= \"899\"\nBlackTitle <> \"BOT\"\nWhiteTitle <> \"BOT\"\n"
	cs, err := ReadCriteria(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	m := &TagMatcher{}
	for _, c := range cs {
		m.Add(c)
	}
	human := game("WhiteElo", "850", "BlackElo", "820", "TimeControl", "180+2")
	if !m.Match(human) {
		t.Error("human game without title tags should match")
	}
	bot := game("WhiteElo", "850", "BlackElo", "820", "TimeControl", "180+2", "BlackTitle", "BOT")
	if m.Match(bot) {
		t.Error("bot game should not match")
	}
	titled := game("WhiteElo", "850", "BlackElo", "820", "TimeControl", "180+2", "BlackTitle", "GM")
	if !m.Match(titled) {
		t.Error("non-BOT title should match")
	}
}

func TestTagMatcherGenericRelational(t *testing.T) {
	// Non-Elo numeric tags go through check_list's relational pass.
	m := matcher(t, `Round > "3"`)
	if !m.Match(game("Round", "5")) || m.Match(game("Round", "2")) {
		t.Error("numeric relational on Round")
	}
	if m.Match(game("Round", "final")) {
		t.Error("non-numeric value with relational only: no range check, fails")
	}
	m = matcher(t, `Round <> "3"`)
	if !m.Match(game("Round", "4")) || m.Match(game("Round", "3")) || !m.Match(game("Round", "final")) {
		t.Error("<> numeric and non-numeric")
	}
	m = matcher(t, `Event <> "World"`)
	if m.Match(game("Event", "World Ch")) || !m.Match(game("Event", "Candidates")) {
		t.Error("<> as not-prefix")
	}
	m = matcher(t, `Event "Cand"`, `Event <> "World"`)
	if !m.Match(game("Event", "Candidates")) || !m.Match(game("Event", "Cand World")) {
		t.Error("prefix match wins before <> is tested")
	}
}

func TestTagMatcherRegex(t *testing.T) {
	m := matcher(t, `Event =~ "^World.*Ch"`)
	if !m.Match(game("Event", "World Championship")) || m.Match(game("Event", "Candidates")) {
		t.Error("regex")
	}
}

func TestTagMatcherSoundex(t *testing.T) {
	m := &TagMatcher{Soundex: true}
	m.AddPrefix("Player", "Petrosian")
	if !m.Match(game("White", "Petrosjan, T.", "Black", "X")) {
		t.Error("soundex player match")
	}
	if m.Match(game("White", "Fischer", "Black", "Spassky")) {
		t.Error("soundex no match")
	}
	// Date is not a soundex tag.
	m = &TagMatcher{Soundex: true}
	m.AddPrefix("Date", "1970")
	if !m.Match(game("Date", "1970.01.01")) {
		t.Error("date unaffected by soundex")
	}
}

func TestTagMatcherECO(t *testing.T) {
	m := matcher(t, `ECO "B9"`)
	if !m.Match(game("White", "x")) {
		t.Error("Match ignores ECO")
	}
	if !m.MatchECO(game("ECO", "B97")) || m.MatchECO(game("ECO", "C50")) || m.MatchECO(game("White", "x")) {
		t.Error("MatchECO")
	}
	if !(&TagMatcher{}).MatchECO(game("White", "x")) {
		t.Error("no ECO criteria")
	}
}

func TestRatedWinner(t *testing.T) {
	cases := []struct {
		g             *pgn.Game
		higher, lower bool
	}{
		{game("WhiteElo", "2700", "BlackElo", "2500", "Result", "1-0"), true, false},
		{game("WhiteElo", "2700", "BlackElo", "2500", "Result", "0-1"), false, true},
		{game("WhiteElo", "2500", "BlackElo", "2500", "Result", "1-0"), false, false},
		{game("WhiteElo", "2700", "BlackElo", "2500", "Result", "1/2-1/2"), false, false},
		{game("WhiteElo", "2700", "BlackElo", "2500", "Result", "*"), false, false},
		{game("WhiteElo", "2700", "Result", "1-0"), false, false},
		{game("WhiteElo", "?", "BlackElo", "2500", "Result", "1-0"), false, false},
	}
	for _, c := range cases {
		if got := RatedWinner(c.g, true, false); got != c.higher {
			t.Errorf("higher %v = %v", c.g.Tags, got)
		}
		if got := RatedWinner(c.g, false, true); got != c.lower {
			t.Errorf("lower %v = %v", c.g.Tags, got)
		}
	}
}
