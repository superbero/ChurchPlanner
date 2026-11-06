package songs

import (
	"reflect"
	"strings"
	"testing"
)

func TestNormalizeTagSlugs(t *testing.T) {
	got := normalizeTagSlugs([]string{"", " ", "worship", " season ", "worship"})
	want := []string{"worship", "season"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeTagSlugs() = %#v, want %#v", got, want)
	}
}

func TestSongSearchConditionsIgnoreEmptyTagValues(t *testing.T) {
	args, where := songSearchConditions(Filter{
		Query:    "grace",
		TagSlugs: []string{"", " "},
	})
	if len(args) != 1 || args[0] != "grace" {
		t.Fatalf("search args = %#v, want only the text query", args)
	}
	if strings.Contains(where, "song_tags") {
		t.Fatalf("search conditions unexpectedly filter by empty tags: %s", where)
	}
}

func TestSongSearchConditionsRequireAllSelectedTags(t *testing.T) {
	args, where := songSearchConditions(Filter{
		TagSlugs: []string{"", "worship", "season"},
	})
	if !strings.Contains(where, "ANY($1)") || !strings.Contains(where, "= $2") {
		t.Fatalf("tag search conditions = %s", where)
	}
	if len(args) != 2 || !reflect.DeepEqual(args[0], []string{"worship", "season"}) || args[1] != 2 {
		t.Fatalf("tag search args = %#v, want normalized slugs and count 2", args)
	}
}
