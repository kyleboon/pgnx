package pgn

import (
	"reflect"
	"testing"
)

// TestSetTagReplacesLastOccurrenceInPlaceOrAppends pins the one tag-setting
// method every package uses: an existing tag keeps its place in the tag
// order (the last occurrence of a duplicated name, which is the one Tag
// reads), and an absent tag is appended, so a game written back out keeps
// its source tag order.
func TestSetTagReplacesLastOccurrenceInPlaceOrAppends(t *testing.T) {
	g := &Game{Tags: []Tag{
		{Name: "Event", Value: "E"},
		{Name: "Result", Value: "1/2"},
		{Name: "Site", Value: "S"},
		{Name: "Result", Value: "?"},
		{Name: "White", Value: "W"},
	}}

	g.SetTag("Result", "1/2-1/2")
	g.SetTag("SetUp", "1")

	want := []Tag{
		{Name: "Event", Value: "E"},
		{Name: "Result", Value: "1/2"},
		{Name: "Site", Value: "S"},
		{Name: "Result", Value: "1/2-1/2"},
		{Name: "White", Value: "W"},
		{Name: "SetUp", Value: "1"},
	}
	if !reflect.DeepEqual(g.Tags, want) {
		t.Errorf("tags after SetTag = %v, want %v", g.Tags, want)
	}
	if v, ok := g.Tag("Result"); !ok || v != "1/2-1/2" {
		t.Errorf("Tag(Result) = %q, %v; want 1/2-1/2, true", v, ok)
	}

	var empty Game
	empty.SetTag("Result", "*")
	if want := []Tag{{Name: "Result", Value: "*"}}; !reflect.DeepEqual(empty.Tags, want) {
		t.Errorf("SetTag on a game with no tags = %v, want %v", empty.Tags, want)
	}
}
