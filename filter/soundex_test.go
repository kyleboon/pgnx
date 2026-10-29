package filter

import "testing"

func TestSoundexPairs(t *testing.T) {
	same := [][2]string{
		{"Nimzovich", "Nimsowitsch"},
		{"Tal", "Talj"},
		{"Yusupov", "Jusupov"},
		{"Petrosian", "Petrosjan"},
		{"fischer", "FISCHER"},
	}
	for _, p := range same {
		if a, b := soundex(p[0]), soundex(p[1]); a != b {
			t.Errorf("soundex(%q)=%q != soundex(%q)=%q", p[0], a, p[1], b)
		}
	}
	diff := [][2]string{
		{"Janosevic", "Nimzovich"},
		{"Fischer", "Petrosian"},
	}
	for _, p := range diff {
		if a, b := soundex(p[0]), soundex(p[1]); a == b {
			t.Errorf("soundex(%q)=%q == soundex(%q)", p[0], a, p[1])
		}
	}
}

func TestSoundexValues(t *testing.T) {
	cases := map[string]string{
		"Nimzovich": "5212",
		"Tal":       "24", // C mapping codes T as 2
		"Yusupov":   "721",
		"":          "",
		"123":       "",
		"Aaron":     "65",
	}
	for in, want := range cases {
		if got := soundex(in); got != want {
			t.Errorf("soundex(%q) = %q, want %q", in, got, want)
		}
	}
}
