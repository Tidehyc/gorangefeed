package moderation

import (
	"time"
)

// ReviewStatus represents the state of a content moderation task.
type ReviewStatus int8

const (
	ReviewStatusPendingMachine ReviewStatus = 1
	ReviewStatusPendingHuman   ReviewStatus = 2
	ReviewStatusApproved       ReviewStatus = 3
	ReviewStatusRejected       ReviewStatus = 4
	ReviewStatusTakedown       ReviewStatus = 5
)

// ReviewPolicy represents whether the author's video goes through pre-review or post-review.
type ReviewPolicy int8

const (
	ReviewPolicyPostReview ReviewPolicy = 1 // 先发后审 (Post-review: published immediately, machine checks asynchronously)
	ReviewPolicyPreReview  ReviewPolicy = 2 // 先审后发 (Pre-review: held in pending until approved)
)

const (
	ActionMachineApprove = "machine_approve"
	ActionMachineReject  = "machine_reject"
	ActionHumanApprove   = "human_approve"
	ActionHumanReject    = "human_reject"
	ActionTakedown       = "takedown"
	ActionBanUser        = "ban_user"
	ActionUnbanUser      = "unban_user"

	TargetTypeVideo = "video"
	TargetTypeUser  = "user"

	HighRiskThreshold = 60
)

// ReviewTask is the aggregate root representing a video moderation task.
type ReviewTask struct {
	ID           int64        `json:"id"`
	VideoID      int64        `json:"video_id"`
	Policy       ReviewPolicy `json:"policy"`
	Status       ReviewStatus `json:"status"`
	RiskScore    int          `json:"risk_score"`
	MatchedWords []string     `json:"matched_words"`
	ReviewerID   int64        `json:"reviewer_id"`
	ReviewNote   string       `json:"review_note"`
	CreatedAt    time.Time    `json:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at"`
}

// NewReviewTask constructs a new ReviewTask with initial pending status.
func NewReviewTask(videoID int64, policy ReviewPolicy) (*ReviewTask, error) {
	if videoID <= 0 {
		return nil, ErrInvalidVideoID
	}
	if policy != ReviewPolicyPostReview && policy != ReviewPolicyPreReview {
		return nil, ErrInvalidPolicy
	}
	now := time.Now().UTC()
	return &ReviewTask{
		VideoID:      videoID,
		Policy:       policy,
		Status:       ReviewStatusPendingMachine,
		RiskScore:    0,
		MatchedWords: []string{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

// ApplyMachineResult applies machine inspection findings to the task.
func (t *ReviewTask) ApplyMachineResult(riskScore int, matchedWords []string) error {
	if riskScore < 0 {
		return ErrInvalidRiskScore
	}
	if t.Status != ReviewStatusPendingMachine {
		return ErrTaskAlreadyProcessed
	}

	t.RiskScore = riskScore
	t.MatchedWords = matchedWords
	t.UpdatedAt = time.Now().UTC()

	if riskScore == 0 {
		t.Status = ReviewStatusApproved
		t.ReviewNote = "machine auto-approved: no sensitive keywords detected"
	} else if riskScore > HighRiskThreshold {
		t.Status = ReviewStatusRejected
		t.ReviewNote = "machine auto-rejected: high risk content detected"
	} else {
		t.Status = ReviewStatusPendingHuman
		t.ReviewNote = "machine flagged: pending human reviewer audit"
	}
	return nil
}

// ApproveByHuman transitions the task to approved by a human moderator.
func (t *ReviewTask) ApproveByHuman(reviewerID int64, note string) error {
	if reviewerID <= 0 {
		return ErrUnauthorizedReviewer
	}
	if t.Status != ReviewStatusPendingHuman && t.Status != ReviewStatusPendingMachine {
		return ErrTaskAlreadyProcessed
	}
	t.Status = ReviewStatusApproved
	t.ReviewerID = reviewerID
	t.ReviewNote = note
	t.UpdatedAt = time.Now().UTC()
	return nil
}

// RejectByHuman transitions the task to rejected by a human moderator.
func (t *ReviewTask) RejectByHuman(reviewerID int64, note string) error {
	if reviewerID <= 0 {
		return ErrUnauthorizedReviewer
	}
	if t.Status != ReviewStatusPendingHuman && t.Status != ReviewStatusPendingMachine {
		return ErrTaskAlreadyProcessed
	}
	t.Status = ReviewStatusRejected
	t.ReviewerID = reviewerID
	t.ReviewNote = note
	t.UpdatedAt = time.Now().UTC()
	return nil
}

// Takedown takes down a previously approved video.
func (t *ReviewTask) Takedown(reviewerID int64, reason string) error {
	if reviewerID <= 0 {
		return ErrUnauthorizedReviewer
	}
	if t.Status != ReviewStatusApproved {
		return ErrTaskAlreadyProcessed
	}
	t.Status = ReviewStatusTakedown
	t.ReviewerID = reviewerID
	t.ReviewNote = reason
	t.UpdatedAt = time.Now().UTC()
	return nil
}

func (t *ReviewTask) IsApproved() bool {
	return t.Status == ReviewStatusApproved
}

func (t *ReviewTask) IsRejected() bool {
	return t.Status == ReviewStatusRejected
}

func (t *ReviewTask) IsPending() bool {
	return t.Status == ReviewStatusPendingMachine || t.Status == ReviewStatusPendingHuman
}

func (t *ReviewTask) IsTakedown() bool {
	return t.Status == ReviewStatusTakedown
}

// ModerationLog represents an audit log entry for moderation activities.
type ModerationLog struct {
	ID         int64     `json:"id"`
	TargetType string    `json:"target_type"`
	TargetID   int64     `json:"target_id"`
	Action     string    `json:"action"`
	OperatorID int64     `json:"operator_id"`
	Reason     string    `json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
}
