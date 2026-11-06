package account

import "context"

// VideoHider is the narrow view of the video domain that account deletion needs.
//
// It exists because account must not import video: the dependency would be circular-adjacent
// (video already depends on account for authorship) and would drag video's whole repository
// surface into this package. main wires the video repository in as the implementation.
type VideoHider interface {
	// HideAllByAuthor hides every published video authored by authorID and reports how many
	// rows changed. It must be idempotent: calling it twice hides nothing extra.
	HideAllByAuthor(ctx context.Context, authorID int64) (int64, error)
}
