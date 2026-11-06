package video

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizeTag(t *testing.T) {
	cases := map[string]string{
		"#旅行":     "旅行",
		"  旅行  ":  "旅行",
		"#Travel": "travel",
		"TRAVEL":  "travel",
		"#":       "",
		"":        "",
	}

	for input, want := range cases {
		if got := NormalizeTag(input); got != want {
			t.Fatalf("NormalizeTag(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestParseTags_SplitsAndDeduplicates(t *testing.T) {
	tags, err := ParseTags([]string{"#旅行 海边,#美食", "旅行", "  "})
	if err != nil {
		t.Fatalf("ParseTags returned error: %v", err)
	}

	want := []string{"旅行", "海边", "美食"}
	if len(tags) != len(want) {
		t.Fatalf("expected %d tags, got %d: %+v", len(want), len(tags), tags)
	}
	for i, tag := range tags {
		if tag.Name != want[i] {
			t.Fatalf("tag %d: expected name %q, got %q", i, want[i], tag.Name)
		}
	}

	// The duplicate "旅行" must not create a second dimension, and the first spelling wins.
	if tags[0].DisplayName != "旅行" {
		t.Fatalf("expected the first spelling to be kept, got %q", tags[0].DisplayName)
	}
}

func TestParseTags_PreservesDisplayCasing(t *testing.T) {
	tags, err := ParseTags([]string{"#Travel"})
	if err != nil {
		t.Fatalf("ParseTags returned error: %v", err)
	}
	if tags[0].Name != "travel" {
		t.Fatalf("expected normalized name %q, got %q", "travel", tags[0].Name)
	}
	if tags[0].DisplayName != "Travel" {
		t.Fatalf("expected display name %q, got %q", "Travel", tags[0].DisplayName)
	}
}

func TestParseTags_RejectsTooMany(t *testing.T) {
	_, err := ParseTags([]string{"a,b,c,d,e,f"})
	if !errors.Is(err, ErrTooManyTags) {
		t.Fatalf("expected ErrTooManyTags, got %v", err)
	}
}

func TestParseTags_RejectsTooLong(t *testing.T) {
	_, err := ParseTags([]string{strings.Repeat("字", MaxTagLength+1)})
	if !errors.Is(err, ErrTagTooLong) {
		t.Fatalf("expected ErrTagTooLong, got %v", err)
	}
}

func TestParseTags_AcceptsExactlyTheLimit(t *testing.T) {
	tags, err := ParseTags([]string{strings.Repeat("字", MaxTagLength)})
	if err != nil {
		t.Fatalf("a tag of exactly %d runes must be accepted, got %v", MaxTagLength, err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(tags))
	}
}

func TestParseTags_EmptyInput(t *testing.T) {
	tags, err := ParseTags(nil)
	if err != nil {
		t.Fatalf("nil input must not be an error, got %v", err)
	}
	if len(tags) != 0 {
		t.Fatalf("expected no tags, got %+v", tags)
	}
}
