package moderation

import (
	"encoding/json"
	"time"

	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
)

// ReviewTaskModel represents the database table `review_tasks`.
type ReviewTaskModel struct {
	ID           int64     `gorm:"primaryKey;autoIncrement"`
	VideoID      int64     `gorm:"column:video_id;not null;uniqueIndex:uk_video_id"`
	Policy       int8      `gorm:"column:policy;not null;default:1"`
	Status       int8      `gorm:"column:status;not null;default:1;index:idx_status_id"`
	RiskScore    int       `gorm:"column:risk_score;not null;default:0"`
	MatchedWords string    `gorm:"column:matched_words;type:json"`
	ReviewerID   int64     `gorm:"column:reviewer_id;not null;default:0;index:idx_reviewer_status"`
	ReviewNote   string    `gorm:"column:review_note;type:varchar(500);not null;default:''"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (ReviewTaskModel) TableName() string {
	return "review_tasks"
}

func toTaskModel(entity *domainmoderation.ReviewTask) *ReviewTaskModel {
	if entity == nil {
		return nil
	}
	wordsBytes, _ := json.Marshal(entity.MatchedWords)
	if entity.MatchedWords == nil {
		wordsBytes = []byte("[]")
	}
	return &ReviewTaskModel{
		ID:           entity.ID,
		VideoID:      entity.VideoID,
		Policy:       int8(entity.Policy),
		Status:       int8(entity.Status),
		RiskScore:    entity.RiskScore,
		MatchedWords: string(wordsBytes),
		ReviewerID:   entity.ReviewerID,
		ReviewNote:   entity.ReviewNote,
		CreatedAt:    entity.CreatedAt,
		UpdatedAt:    entity.UpdatedAt,
	}
}

func toDomainTask(m *ReviewTaskModel) *domainmoderation.ReviewTask {
	if m == nil {
		return nil
	}
	var words []string
	if m.MatchedWords != "" {
		_ = json.Unmarshal([]byte(m.MatchedWords), &words)
	}
	if words == nil {
		words = []string{}
	}
	return &domainmoderation.ReviewTask{
		ID:           m.ID,
		VideoID:      m.VideoID,
		Policy:       domainmoderation.ReviewPolicy(m.Policy),
		Status:       domainmoderation.ReviewStatus(m.Status),
		RiskScore:    m.RiskScore,
		MatchedWords: words,
		ReviewerID:   m.ReviewerID,
		ReviewNote:   m.ReviewNote,
		CreatedAt:    m.CreatedAt,
		UpdatedAt:    m.UpdatedAt,
	}
}

// ModerationLogModel represents the database table `moderation_logs`.
type ModerationLogModel struct {
	ID         int64     `gorm:"primaryKey;autoIncrement"`
	TargetType string    `gorm:"column:target_type;type:varchar(32);not null"`
	TargetID   int64     `gorm:"column:target_id;not null"`
	Action     string    `gorm:"column:action;type:varchar(32);not null"`
	OperatorID int64     `gorm:"column:operator_id;not null;default:0"`
	Reason     string    `gorm:"column:reason;type:varchar(500);not null;default:''"`
	CreatedAt  time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (ModerationLogModel) TableName() string {
	return "moderation_logs"
}

func toLogModel(entity *domainmoderation.ModerationLog) *ModerationLogModel {
	if entity == nil {
		return nil
	}
	return &ModerationLogModel{
		ID:         entity.ID,
		TargetType: entity.TargetType,
		TargetID:   entity.TargetID,
		Action:     entity.Action,
		OperatorID: entity.OperatorID,
		Reason:     entity.Reason,
		CreatedAt:  entity.CreatedAt,
	}
}

func toDomainLog(m *ModerationLogModel) *domainmoderation.ModerationLog {
	if m == nil {
		return nil
	}
	return &domainmoderation.ModerationLog{
		ID:         m.ID,
		TargetType: m.TargetType,
		TargetID:   m.TargetID,
		Action:     m.Action,
		OperatorID: m.OperatorID,
		Reason:     m.Reason,
		CreatedAt:  m.CreatedAt,
	}
}
