package video

import (
	"context"
	"fmt"
	"time"

	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	"gorm.io/gorm"
)

// TagModel maps the tag dictionary. Name is the normalized lookup key; DisplayName is what the UI
// renders.
type TagModel struct {
	ID          int64     `gorm:"primaryKey;autoIncrement"`
	Name        string    `gorm:"column:name"`
	DisplayName string    `gorm:"column:display_name"`
	CreatedAt   time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (TagModel) TableName() string { return "tags" }

// VideoTagModel maps the video-tag relation. Weight is reserved for the M11-02 similarity score;
// everything is written as 1.0 until tag ranking exists.
type VideoTagModel struct {
	VideoID   int64     `gorm:"column:video_id;primaryKey"`
	TagID     int64     `gorm:"column:tag_id;primaryKey"`
	Weight    float64   `gorm:"column:weight"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (VideoTagModel) TableName() string { return "video_tags" }

// defaultTagWeight is written for now; M11-02 will derive real weights from position and title
// matches. Keeping the column populated means that work needs no migration.
const defaultTagWeight = 1.0

// upsertTagTx returns the dictionary ID for a tag, creating it on first use. Two statements rather
// than LAST_INSERT_ID trickery: the INSERT stays idempotent under the unique key, and the SELECT
// then reads whichever row won, so concurrent publishers of the same new tag both get a valid ID.
func upsertTagTx(tx *gorm.DB, tag domainvideo.Tag) (int64, error) {
	err := tx.Exec(`INSERT INTO tags (name, display_name) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE display_name = display_name`, tag.Name, tag.DisplayName).Error
	if err != nil {
		return 0, fmt.Errorf("upsert tag %q error: %w", tag.Name, err)
	}

	var id int64
	if err := tx.Raw(`SELECT id FROM tags WHERE name = ?`, tag.Name).Scan(&id).Error; err != nil {
		return 0, fmt.Errorf("load tag %q error: %w", tag.Name, err)
	}
	if id == 0 {
		return 0, fmt.Errorf("tag %q vanished after upsert", tag.Name)
	}
	return id, nil
}

// replaceVideoTagsTx rewrites the tag links of one video inside the caller's transaction. Delete
// first so a re-publish with fewer tags cannot leave stale links behind.
func replaceVideoTagsTx(tx *gorm.DB, videoID int64, tags []domainvideo.Tag) error {
	if err := tx.Where("video_id = ?", videoID).Delete(&VideoTagModel{}).Error; err != nil {
		return fmt.Errorf("clear video tags error: %w", err)
	}
	if len(tags) == 0 {
		return nil
	}

	rows := make([]VideoTagModel, 0, len(tags))
	for _, tag := range tags {
		tagID, err := upsertTagTx(tx, tag)
		if err != nil {
			return err
		}
		rows = append(rows, VideoTagModel{VideoID: videoID, TagID: tagID, Weight: defaultTagWeight})
	}

	if err := tx.Create(&rows).Error; err != nil {
		return fmt.Errorf("create video tags error: %w", err)
	}
	return nil
}

type videoTagRow struct {
	VideoID     int64  `gorm:"column:video_id"`
	Name        string `gorm:"column:name"`
	DisplayName string `gorm:"column:display_name"`
}

// loadTagsByVideoIDs returns normalized name and display name per video, ordered by weight so the
// strongest tag renders first.
func loadTagsByVideoIDs(ctx context.Context, db *gorm.DB, videoIDs []int64) (map[int64][]domainvideo.Tag, error) {
	out := make(map[int64][]domainvideo.Tag, len(videoIDs))
	if len(videoIDs) == 0 {
		return out, nil
	}

	var rows []videoTagRow
	err := db.WithContext(ctx).Raw(`
		SELECT vt.video_id, t.name, t.display_name
		FROM video_tags vt
		JOIN tags t ON t.id = vt.tag_id
		WHERE vt.video_id IN (?)
		ORDER BY vt.video_id, vt.weight DESC, t.id`, videoIDs).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load video tags error: %w", err)
	}

	for _, row := range rows {
		out[row.VideoID] = append(out[row.VideoID], domainvideo.Tag{
			Name:        row.Name,
			DisplayName: row.DisplayName,
		})
	}
	return out, nil
}
