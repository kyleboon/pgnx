package filter

// maxSoundex bounds the soundex result length (lists.c MAXSOUNDEX).
const maxSoundex = 50

// soundexMapping maps A..Z to their soundex code, '0' meaning "ignored".
const soundexMapping = "01230120002455012622011202"

// soundex is an exact port of pgn-extract's soundex (lists.c): a
// deliberately tolerant variant of the classic algorithm. A leading J or
// Y becomes '7' (so Janosevic != Nimzovich while Yusupov == Jusupov);
// only ASCII letters contribute; letters coding to '0' are dropped
// without resetting the run; and a code equal to the previous emitted
// code is dropped. Unlike classic soundex the result is not padded or
// truncated to four characters.
func soundex(s string) string {
	var out []byte
	i := 0
	if len(s) > 0 {
		initial := upperASCII(s[0])
		if initial == 'Y' || initial == 'J' {
			out = append(out, '7')
			i = 1
		}
	}
	lastc := byte(' ')
	for ; i < len(s) && len(out) < maxSoundex; i++ {
		ch := s[i]
		if !isASCIIAlpha(ch) || ch == lastc {
			continue
		}
		ch = upperASCII(ch)
		tr := soundexMapping[ch-'A']
		if tr != '0' && tr != lastc {
			out = append(out, tr)
			lastc = tr
		}
	}
	return string(out)
}

func isASCIIAlpha(ch byte) bool {
	return ch >= 'A' && ch <= 'Z' || ch >= 'a' && ch <= 'z'
}

func upperASCII(ch byte) byte {
	if ch >= 'a' && ch <= 'z' {
		return ch - 'a' + 'A'
	}
	return ch
}

// soundexTag reports whether soundex matching applies to criteria on the
// named tag when TagMatcher.Soundex is set (lists.c soundex_tag).
func soundexTag(tag string) bool {
	switch tag {
	case "White", "Black", "Player", "Event", "Site", "Annotator":
		return true
	}
	return false
}
