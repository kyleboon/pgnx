package filter

import "strconv"

// parseFloatPrefix parses s, which scanFloat has already trimmed to a
// syntactically valid float, with strconv.
func parseFloatPrefix(s string) (float64, error) {
	return strconv.ParseFloat(s, 64)
}
