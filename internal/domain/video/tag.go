package video

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const (
	// MaxTagsPerVideo caps how many tags one video may carry. Five is what short-video products
	// converge on: enough to describe a clip, few enough that a tag vector stays interpretable.
	MaxTagsPerVideo = 5
	// MaxTagLength bounds a single tag in runes, not bytes, so Chinese tags are not penalised.
	MaxTagLength = 32
)

var (
	ErrTooManyTags = errors.New("a video can carry at most 5 tags")
	ErrTagTooLong  = errors.New("a tag cannot exceed 32 characters")
)

// Tag is one dictionary entry. Name is the normalized lookup key and is what the algorithm uses;
// DisplayName keeps the original casing and is what the UI renders.
type Tag struct {
	Name        string `json:"name"`
	DisplayName string `json:"display_name"`
}

// NormalizeTag turns free-form input into a dictionary key: '#' markers and surrounding spaces are
// stripped and the text is lowercased, so "#旅行", "旅行" and "旅行 " collapse into one dimension.
func NormalizeTag(raw string) string {
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(raw), "#")))
}

// ParseTags normalizes a free-form tag list: it splits on '#'/whitespace/commas, de-duplicates by
// normalized name while keeping the first spelling seen, and enforces the count and length limits.
// Returning an error instead of silently truncating matters: a dropped tag changes the video's
// feature vector without telling the author.
func ParseTags(raw []string) ([]Tag, error) {
	tags := make([]Tag, 0, len(raw))
	seen := make(map[string]struct{}, len(raw))

	for _, entry := range raw {
		for _, token := range splitTagTokens(entry) {
			name := NormalizeTag(token)
			if name == "" {
				continue
			}
			if utf8.RuneCountInString(name) > MaxTagLength {
				return nil, ErrTagTooLong
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			tags = append(tags, Tag{
				Name:        name,
				DisplayName: strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(token), "#")),
			})
		}
	}

	if len(tags) > MaxTagsPerVideo {
		return nil, ErrTooManyTags
	}

	return tags, nil
}

// splitTagTokens breaks "旅行 海边,#美食" into ["旅行", "海边", "美食"].
func splitTagTokens(entry string) []string {
	return strings.FieldsFunc(entry, func(r rune) bool {
		switch r {
		case '#', ',', '，', '、', ' ', '\t', '\n', '\r':
			return true
		}
		return false
	})
}

// TagNames projects a tag list onto its normalized names.
func TagNames(tags []Tag) []string {
	names := make([]string, 0, len(tags))
	for _, t := range tags {
		names = append(names, t.Name)
	}
	return names
}
