package video

import (
	"strings"
	"testing"
)

func TestVideo_Validate(t *testing.T) {
	longTitle := strings.Repeat("a", MaxTitleLength+1)
	longDesc := strings.Repeat("d", MaxDescriptionLength+1)
	validTitle := "My First Short Video"
	validPlayURL := "http://example.com/media/videos/123.mp4"

	tests := []struct {
		name    string
		video   Video
		wantErr error
	}{
		{
			name: "valid video",
			video: Video{
				AuthorID: 1,
				Title:    validTitle,
				PlayURL:  validPlayURL,
			},
			wantErr: nil,
		},
		{
			name: "invalid author id",
			video: Video{
				AuthorID: 0,
				Title:    validTitle,
				PlayURL:  validPlayURL,
			},
			wantErr: ErrInvalidAuthorID,
		},
		{
			name: "empty title",
			video: Video{
				AuthorID: 1,
				Title:    "   ",
				PlayURL:  validPlayURL,
			},
			wantErr: ErrEmptyTitle,
		},
		{
			name: "title too long",
			video: Video{
				AuthorID: 1,
				Title:    longTitle,
				PlayURL:  validPlayURL,
			},
			wantErr: ErrTitleTooLong,
		},
		{
			name: "description too long",
			video: Video{
				AuthorID:    1,
				Title:       validTitle,
				Description: longDesc,
				PlayURL:     validPlayURL,
			},
			wantErr: ErrDescriptionTooLong,
		},
		{
			name: "empty play_url",
			video: Video{
				AuthorID: 1,
				Title:    validTitle,
				PlayURL:  "",
			},
			wantErr: ErrEmptyPlayURL,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.video.Validate()
			if tc.wantErr == nil && err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if tc.wantErr != nil && err != tc.wantErr {
				t.Fatalf("expected error %v, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestVideo_StatusHelpers(t *testing.T) {
	v := &Video{Status: VideoStatusPublished}
	if !v.IsPublished() {
		t.Fatal("expected IsPublished to be true")
	}
	if v.IsHidden() {
		t.Fatal("expected IsHidden to be false")
	}

	v.Hide()
	if !v.IsHidden() {
		t.Fatal("expected IsHidden to be true")
	}
	if v.IsPublished() {
		t.Fatal("expected IsPublished to be false")
	}
}
