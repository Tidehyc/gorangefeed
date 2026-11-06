package feed

import (
	"context"
	"fmt"
	"math"

	"gorm.io/gorm"
)

// tagRepository is the read model for video tags. It lives in the feed persistence package rather
// than the video one because both of its methods exist to serve feed assembly, and it keeps the
// video repository's write-side transaction as the single owner of tag mutation.
type tagRepository struct {
	db *gorm.DB
}

// NewTagRepository builds the tag read model used by feed assembly and tag feeds.
func NewTagRepository(db *gorm.DB) *tagRepository {
	return &tagRepository{db: db}
}

type tagNameRow struct {
	VideoID     int64  `gorm:"column:video_id"`
	DisplayName string `gorm:"column:display_name"`
}

// ListVideoTags returns display names per video ID, strongest tag first.
func (r *tagRepository) ListVideoTags(ctx context.Context, videoIDs []int64) (map[int64][]string, error) {
	out := make(map[int64][]string, len(videoIDs))
	if len(videoIDs) == 0 {
		return out, nil
	}

	var rows []tagNameRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT vt.video_id, t.display_name
		FROM video_tags vt
		JOIN tags t ON t.id = vt.tag_id
		WHERE vt.video_id IN (?)
		ORDER BY vt.video_id, vt.weight DESC, t.id`, videoIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list video tags error: %w", err)
	}

	for _, row := range rows {
		out[row.VideoID] = append(out[row.VideoID], row.DisplayName)
	}
	return out, nil
}

// ListVideoIDsByTag pages through the published videos of one tag using a keyset on video_id.
//
// The tag is resolved to its dictionary ID first so the page query can constrain `tag_id` as a
// constant: that lets MySQL use idx_tag_video (tag_id, video_id DESC) instead of scanning the
// primary key backwards and filtering. The absent cursor is sentinel-ed to MaxInt64 rather than
// expressed as `(? = 0 OR video_id < ?)`, because the OR form makes the range unprovable and the
// optimiser falls back to a full reverse scan.
func (r *tagRepository) ListVideoIDsByTag(ctx context.Context, tagName string, cursor int64, limit int) ([]int64, int64, bool, error) {
	if limit <= 0 {
		limit = 10
	}
	if cursor <= 0 {
		cursor = math.MaxInt64
	}

	var tagID int64
	err := r.db.WithContext(ctx).Raw(`SELECT id FROM tags WHERE name = ?`, tagName).Scan(&tagID).Error
	if err != nil {
		return nil, 0, false, fmt.Errorf("resolve tag %q error: %w", tagName, err)
	}
	if tagID == 0 {
		return []int64{}, 0, false, nil
	}

	ids := make([]int64, 0, limit+1)
	err = r.db.WithContext(ctx).Raw(`
		SELECT vt.video_id
		FROM video_tags vt
		JOIN videos v ON v.id = vt.video_id
		WHERE vt.tag_id = ? AND v.status = 1 AND vt.video_id < ?
		ORDER BY vt.video_id DESC
		LIMIT ?`, tagID, cursor, limit+1).Scan(&ids).Error
	if err != nil {
		return nil, 0, false, fmt.Errorf("list video ids by tag error: %w", err)
	}

	hasMore := len(ids) > limit
	if hasMore {
		ids = ids[:limit]
	}
	nextCursor := int64(0)
	if hasMore && len(ids) > 0 {
		nextCursor = ids[len(ids)-1]
	}
	return ids, nextCursor, hasMore, nil
}
