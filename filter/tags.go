package filter

import (
	"fmt"
	"math"
	"strings"

	"github.com/kyleboon/pgnx/pgn"
)

// TagMatcher holds tag criteria and tests games against them, following
// pgn-extract's lists.c: criteria on the same tag are alternatives
// (ORed, with the numeric exceptions noted on Match), criteria on
// different tags are all required (ANDed).
type TagMatcher struct {
	// Substring makes plain criteria match anywhere in the tag value
	// instead of only at its start (--tagsubstr).
	Substring bool
	// Soundex compares White, Black, Player, Event, Site and Annotator
	// criteria by soundex code instead of text (-S).
	Soundex bool

	groups map[string][]Criterion
	order  []string // tag names in first-insertion order
}

// Add appends c to the criteria for c.Tag. A positional criterion (FEN,
// FENPattern, FENPatternI; see Criterion.IsPositional) is rejected with
// an error wrapping ErrPositionalCriterion: it selects by position and
// belongs in a PositionCriteria.
func (m *TagMatcher) Add(c Criterion) error {
	if c.IsPositional() {
		return fmt.Errorf("%s: %w", c.Tag, ErrPositionalCriterion)
	}
	if m.groups == nil {
		m.groups = make(map[string][]Criterion)
	}
	if _, ok := m.groups[c.Tag]; !ok {
		m.order = append(m.order, c.Tag)
	}
	m.groups[c.Tag] = append(m.groups[c.Tag], c)
	return nil
}

// AddPrefix adds a plain prefix criterion, the -T shorthand (e.g. -Tp
// becomes AddPrefix("Player", value)).
func (m *TagMatcher) AddPrefix(tag, value string) {
	m.Add(Criterion{Tag: tag, Op: OpPrefix, Value: value})
}

// Empty reports whether no criteria have been added.
func (m *TagMatcher) Empty() bool {
	return m == nil || len(m.groups) == 0
}

// hasECO reports whether an ECO group exists (checked last, by MatchECO).
func (m *TagMatcher) hasECO() bool {
	return m != nil && len(m.groups["ECO"]) > 0
}

// Match reports whether g satisfies every non-ECO criterion group
// (lists.c check_tag_details_not_ECO). Per-tag rules:
//
//   - Plain (OpPrefix) criteria: any one matching by prefix (or substring
//     when Substring) succeeds. Then any OpRegex criterion matching
//     succeeds. Otherwise, if any <> criterion exists or the tag value is
//     numeric and a relational criterion exists, all relational criteria
//     must hold (numerically; <> on a non-numeric value means "no
//     prefix/substring match").
//   - A tag that is absent from the game fails its group unless every
//     criterion in the group is <>.
//   - Player tests the White tag and then the Black tag against the group.
//     Elo likewise tests WhiteElo then BlackElo; EloDiff tests the absolute
//     difference of the two (both required).
//   - Date, WhiteElo, BlackElo and TimeControl have their own numeric
//     semantics; see matchDate, matchElo and matchTimeControl.
//
// The ECO group is deliberately not tested here; see MatchECO.
func (m *TagMatcher) Match(g *pgn.Game) bool {
	if m.Empty() {
		return true
	}
	wanted := true

	if list := m.groups["Player"]; len(list) > 0 {
		white, hasWhite := g.Tag("White")
		black, hasBlack := g.Tag("Black")
		switch {
		case hasWhite:
			wanted = m.checkList("White", white, list)
			if !wanted && hasBlack {
				wanted = m.checkList("Black", black, list)
			}
		case hasBlack:
			wanted = m.checkList("Black", black, list)
		default:
			wanted = false
		}
	}

	if list := m.groups["Elo"]; len(list) > 0 {
		// NB: this assigns rather than ANDs, exactly as lists.c:933 does,
		// so a passing Elo group overrides a failed Player group. Pinned by
		// TestTagMatcherEloOverridesPlayer; do not "fix" without the oracle.
		white, hasWhite := g.Tag("WhiteElo")
		black, hasBlack := g.Tag("BlackElo")
		switch {
		case hasWhite:
			wanted = matchElo(white, list)
			if !wanted && hasBlack {
				wanted = matchElo(black, list)
			}
		case hasBlack:
			wanted = matchElo(black, list)
		default:
			wanted = false
		}
	}

	if list := m.groups["EloDiff"]; wanted && len(list) > 0 {
		white, hasWhite := g.Tag("WhiteElo")
		black, hasBlack := g.Tag("BlackElo")
		if hasWhite && hasBlack {
			wanted = matchEloDiff(white, black, list)
		} else {
			wanted = false
		}
	}

	for _, tag := range m.order {
		if !wanted {
			break
		}
		switch tag {
		case "Player", "Elo", "EloDiff", "ECO":
			continue
		}
		list := m.groups[tag]
		value, ok := g.Tag(tag)
		if !ok {
			wanted = allNegative(list)
			continue
		}
		switch tag {
		case "Date":
			wanted = matchDate(value, list)
		case "WhiteElo", "BlackElo":
			wanted = matchElo(value, list)
		case "TimeControl":
			wanted = matchTimeControl(value, list)
		default:
			wanted = m.checkList(tag, value, list)
		}
	}
	return wanted
}

// MatchECO tests only the ECO criterion group (lists.c check_ECO_tag).
// It is evaluated last in the C tool's chain so that ECO classification
// (a later phase) can add the tag first; a missing ECO tag always fails
// when there are ECO criteria (no all-negative exception).
func (m *TagMatcher) MatchECO(g *pgn.Game) bool {
	if !m.hasECO() {
		return true
	}
	value, ok := g.Tag("ECO")
	if !ok {
		return false
	}
	return m.checkList("ECO", value, m.groups["ECO"])
}

// allNegative reports whether every criterion is <> (lists.c
// all_negative_matches), which lets an absent tag pass.
func allNegative(list []Criterion) bool {
	for _, c := range list {
		if c.Op != OpNE {
			return false
		}
	}
	return true
}

// checkList is the generic matcher (lists.c check_list); see Match for
// the rules. tag selects whether soundex applies.
func (m *TagMatcher) checkList(tag, value string, list []Criterion) bool {
	useSoundex := m.Soundex && soundexTag(tag)
	search := value
	if useSoundex {
		search = soundex(value)
	}
	crit := func(c Criterion) string {
		if useSoundex {
			return soundex(c.Value)
		}
		return c.Value
	}
	numeric := looksNumeric(search)

	rangeCheck, regexCheck := false, false
	for _, c := range list {
		switch {
		case c.Op == OpPrefix:
			if m.textMatch(search, crit(c)) {
				return true
			}
		case c.Op == OpNE:
			rangeCheck = true
		case c.Op == OpRegex:
			regexCheck = true
		case !rangeCheck && numeric:
			rangeCheck = true
		}
	}
	if regexCheck {
		for _, c := range list {
			if c.Op == OpRegex && c.re != nil && c.re.MatchString(search) {
				return true
			}
		}
	}
	if !rangeCheck {
		return false
	}
	wanted := true
	for _, c := range list {
		if !wanted {
			break
		}
		switch c.Op {
		case OpLT, OpGT, OpLE, OpGE, OpEQ:
			if numeric {
				tv, ok1 := scanFloat(search)
				lv, ok2 := scanFloat(c.Value)
				if ok1 && ok2 {
					wanted = compareNumeric(c.Op, tv, lv)
				}
			}
		case OpNE:
			if numeric {
				tv, ok1 := scanFloat(search)
				lv, ok2 := scanFloat(c.Value)
				if ok1 && ok2 {
					wanted = compareNumeric(c.Op, tv, lv)
				}
			} else {
				wanted = !m.textMatch(search, crit(c))
			}
		}
	}
	return wanted
}

// textMatch applies the plain-criterion comparison: prefix, or substring
// when Substring is set.
func (m *TagMatcher) textMatch(value, want string) bool {
	if m.Substring {
		return strings.Contains(value, want)
	}
	return strings.HasPrefix(value, want)
}

// looksNumeric mirrors check_list's test: optional sign then only digits
// and dots (so "1.2.3" and "" both count as numeric).
func looksNumeric(s string) bool {
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		s = s[1:]
	}
	for i := 0; i < len(s); i++ {
		if ch := s[i]; !(ch >= '0' && ch <= '9') && ch != '.' {
			return false
		}
	}
	return true
}

// compareNumeric is lists.c relative_numeric_match.
func compareNumeric(op Op, lhs, rhs float64) bool {
	switch op {
	case OpLT:
		return lhs < rhs
	case OpLE:
		return lhs <= rhs
	case OpGT:
		return lhs > rhs
	case OpGE:
		return lhs >= rhs
	case OpEQ:
		return lhs == rhs
	case OpNE:
		return lhs != rhs
	}
	return false
}

// matchDate is lists.c check_date. The game's date encodes as
// year*10000+month*100+day (month/day default 1; an unparsable year
// fails). A criterion value may start with 'b' (before, <) or 'a'
// (after, >), which overrides its operator. Relational criteria require
// 100 < year < 3000 and are ANDed; plain criteria are prefix matches,
// evaluated only for the first criterion or while nothing has matched
// yet (the C code's order-dependent mixing, ported as-is).
func matchDate(date string, list []Criterion) bool {
	gameYear, ok := scanUint(date)
	if !ok {
		return false
	}
	gameMonth, gameDay := scanMonthDay(date)
	encodedGame := float64(10000*gameYear + 100*gameMonth + gameDay)

	wanted := false
	for i, c := range list {
		op, value := c.Op, c.Value
		if strings.HasPrefix(value, "b") {
			op, value = OpLT, value[1:]
		} else if strings.HasPrefix(value, "a") {
			op, value = OpGT, value[1:]
		}
		if op != OpPrefix {
			listYear, ok := scanUint(value)
			if !ok {
				wanted = false
				continue
			}
			if gameYear > minDate && gameYear < maxDate {
				listMonth, listDay := scanMonthDay(value)
				encodedList := float64(10000*listYear + 100*listMonth + listDay)
				matches := compareNumeric(op, encodedGame, encodedList)
				if i == 0 {
					wanted = matches
				} else {
					wanted = wanted && matches
				}
			} else {
				wanted = false
			}
		} else if i == 0 || !wanted {
			wanted = strings.HasPrefix(date, value)
		}
	}
	return wanted
}

// Year limits for relational date comparisons (lists.c MINDATE/MAXDATE).
const (
	minDate = 100
	maxDate = 3000
)

// scanMonthDay extracts the ".M.D" part of a date as sscanf("%*u.%u.%u")
// would: each field defaults to 1 and parsing stops at the first field
// that fails (so "1962.??.??" yields 1, 1).
func scanMonthDay(s string) (month, day int) {
	month, day = 1, 1
	i := skipUint(s, 0)
	if i >= len(s) || s[i] != '.' {
		return
	}
	m, n := scanUintAt(s, i+1)
	if n == 0 {
		return
	}
	month = m
	i += 1 + n
	if i >= len(s) || s[i] != '.' {
		return
	}
	d, n := scanUintAt(s, i+1)
	if n == 0 {
		return
	}
	day = d
	return
}

// skipUint returns the index just past a leading unsigned integer at i
// (after optional whitespace and sign), or i if there is none.
func skipUint(s string, i int) int {
	_, n := scanUintAt(s, i)
	return i + n
}

// scanUintAt parses an unsigned decimal integer at s[i:] the way
// sscanf("%u") does: leading whitespace and an optional sign are
// consumed, then one or more digits. It returns the value and the number
// of bytes consumed (0 when no digits were found).
func scanUintAt(s string, i int) (int, int) {
	j := i
	for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
		j++
	}
	if j < len(s) && (s[j] == '+' || s[j] == '-') {
		j++
	}
	start := j
	v := 0
	for j < len(s) && s[j] >= '0' && s[j] <= '9' {
		if v < math.MaxInt32 {
			v = v*10 + int(s[j]-'0')
		}
		j++
	}
	if j == start {
		return 0, 0
	}
	return v, j - i
}

// scanUint parses a leading unsigned integer (sscanf "%u" semantics).
func scanUint(s string) (int, bool) {
	v, n := scanUintAt(s, 0)
	return v, n > 0
}

// scanFloat parses a leading floating-point number (sscanf "%lf"
// semantics: optional whitespace, sign, digits, optional fraction and
// exponent; trailing text ignored).
func scanFloat(s string) (float64, bool) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	start := i
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return 0, false
	}
	end := i
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		k := j
		for k < len(s) && s[k] >= '0' && s[k] <= '9' {
			k++
		}
		if k > j {
			end = k
		}
	}
	v, err := parseFloatPrefix(s[start:end])
	if err != nil {
		return 0, false
	}
	return v, true
}

// matchElo is lists.c check_elo: the value must parse as an unsigned
// integer; if any relational criterion (with a parsable value) exists,
// all relational criteria must hold and plain criteria are ignored;
// otherwise any plain criterion matching by prefix succeeds.
func matchElo(elo string, list []Criterion) bool {
	gameElo, ok := scanUint(elo)
	if !ok {
		return false
	}
	wanted := true
	relationalFound := false
	for _, c := range list {
		if !wanted {
			break
		}
		if c.Op == OpPrefix {
			continue
		}
		if listElo, ok := scanUint(c.Value); ok {
			relationalFound = true
			wanted = compareNumeric(c.Op, float64(gameElo), float64(listElo))
		}
	}
	if relationalFound {
		return wanted
	}
	for _, c := range list {
		if c.Op == OpPrefix && strings.HasPrefix(elo, c.Value) {
			return true
		}
	}
	return false
}

// matchEloDiff is lists.c check_elo_diff: |WhiteElo-BlackElo| must satisfy
// every relational criterion whose value is numeric; plain criteria are
// ignored.
func matchEloDiff(white, black string, list []Criterion) bool {
	w, ok1 := scanInt(white)
	b, ok2 := scanInt(black)
	if !ok1 || !ok2 {
		return false
	}
	diff := math.Abs(float64(w - b))
	wanted := true
	for _, c := range list {
		if !wanted {
			break
		}
		if !c.Op.relational() {
			continue
		}
		if lv, ok := scanFloat(c.Value); ok {
			wanted = compareNumeric(c.Op, diff, lv)
		}
	}
	return wanted
}

// scanInt parses a leading signed integer (sscanf "%d" semantics).
func scanInt(s string) (int, bool) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	neg := false
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		neg = s[i] == '-'
		i++
	}
	v, n := scanUintAt(s, i)
	if n == 0 {
		return 0, false
	}
	if neg {
		v = -v
	}
	return v, true
}

// matchTimeControl is lists.c check_time_control: the first control
// (before any ':') is reduced to a period in seconds — "N+inc" and "*N"
// give N, "moves/secs" gives secs, bare digits give N — and that period
// must satisfy any one criterion (relational ones compare the period,
// plain ones prefix-match the control text). "", "?" and "-" never match.
func matchTimeControl(tc string, list []Criterion) bool {
	if tc == "" || tc == "?" || tc == "-" {
		return false
	}
	control := tc
	if i := strings.IndexByte(control, ':'); i >= 0 {
		control = control[:i]
	}
	var period int
	var ok bool
	switch {
	case strings.Contains(control, "+"):
		period, ok = scanUint(control)
	case strings.HasPrefix(control, "*"):
		period, ok = scanUint(control[1:])
	case strings.Contains(control, "/"):
		period, ok = scanUint(control[strings.IndexByte(control, '/')+1:])
	case len(control) > 0 && control[0] >= '0' && control[0] <= '9':
		allDigits := true
		for i := 0; i < len(control); i++ {
			if control[i] < '0' || control[i] > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			period, ok = scanUint(control)
		}
	}
	if !ok {
		return false
	}
	for _, c := range list {
		if c.Op != OpPrefix {
			if lv, ok := scanUint(c.Value); ok && compareNumeric(c.Op, float64(period), float64(lv)) {
				return true
			}
		} else if strings.HasPrefix(control, c.Value) {
			return true
		}
	}
	return false
}

// RatedWinner is lists.c check_Elo_difference: with higher set the game
// must have been won (Result exactly "1-0" or "0-1") by the player with
// the strictly higher Elo; with lower set, by the strictly lower. Both
// Elo tags and the Result tag are required, and draws and equal ratings
// never match.
func RatedWinner(g *pgn.Game, higher, lower bool) bool {
	white, ok1 := g.Tag("WhiteElo")
	black, ok2 := g.Tag("BlackElo")
	result, ok3 := g.Tag("Result")
	if !ok1 || !ok2 || !ok3 {
		return false
	}
	w, ok := scanInt(white)
	if !ok {
		return false
	}
	b, ok := scanInt(black)
	if !ok {
		return false
	}
	var diff int
	switch result {
	case "1-0":
		diff = w - b
	case "0-1":
		diff = b - w
	default:
		return false
	}
	return (diff > 0 && higher) || (diff < 0 && lower)
}
