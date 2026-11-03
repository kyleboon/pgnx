package transform

import (
	"bufio"
	"io"
	"strings"
)

// PlayerInfo maps a player's name, exactly as the White or Black tag
// spells it, to a rating or an ID.
type PlayerInfo map[string]string

// LoadPlayerInfo reads pgn-extract's player file format (csvreader.c): one
// player per line, name and value separated by tabs. Runs of tabs count
// as one and a line without exactly two fields is skipped; a later line
// for the same name replaces an earlier one.
func LoadPlayerInfo(r io.Reader) (PlayerInfo, error) {
	info := PlayerInfo{}
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := strings.TrimRight(s.Text(), "\r")
		fields := strings.FieldsFunc(line, func(c rune) bool { return c == '\t' })
		if len(fields) != 2 {
			continue
		}
		info[fields[0]] = fields[1]
	}
	return info, s.Err()
}
