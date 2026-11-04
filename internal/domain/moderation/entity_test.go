package moderation

import (
	"testing"
)

func TestNewReviewTask_Validation(t *testing.T) {
	_, err := NewReviewTask(0, ReviewPolicyPostReview)
	if err != ErrInvalidVideoID {
		t.Fatalf("expected ErrInvalidVideoID, got %v", err)
	}

	_, err = NewReviewTask(100, ReviewPolicy(99))
	if err != ErrInvalidPolicy {
		t.Fatalf("expected ErrInvalidPolicy, got %v", err)
	}

	task, err := NewReviewTask(100, ReviewPolicyPostReview)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.VideoID != 100 || task.Policy != ReviewPolicyPostReview || task.Status != ReviewStatusPendingMachine {
		t.Fatalf("unexpected task state: %+v", task)
	}
	if !task.IsPending() {
		t.Fatalf("expected IsPending() == true")
	}
}

func TestReviewTask_ApplyMachineResult(t *testing.T) {
	task, _ := NewReviewTask(101, ReviewPolicyPostReview)

	// Negative risk score
	if err := task.ApplyMachineResult(-1, nil); err != ErrInvalidRiskScore {
		t.Fatalf("expected ErrInvalidRiskScore, got %v", err)
	}

	// Clean -> Approved
	if err := task.ApplyMachineResult(0, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task.Status != ReviewStatusApproved || !task.IsApproved() {
		t.Fatalf("expected Approved status, got %v", task.Status)
	}

	// Repeated apply should fail
	if err := task.ApplyMachineResult(0, nil); err != ErrTaskAlreadyProcessed {
		t.Fatalf("expected ErrTaskAlreadyProcessed, got %v", err)
	}

	// High risk -> Rejected
	task2, _ := NewReviewTask(102, ReviewPolicyPostReview)
	if err := task2.ApplyMachineResult(85, []string{"prohibited"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task2.Status != ReviewStatusRejected || !task2.IsRejected() {
		t.Fatalf("expected Rejected status, got %v", task2.Status)
	}

	// Moderate risk -> PendingHuman
	task3, _ := NewReviewTask(103, ReviewPolicyPostReview)
	if err := task3.ApplyMachineResult(40, []string{"warning"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if task3.Status != ReviewStatusPendingHuman || !task3.IsPending() {
		t.Fatalf("expected PendingHuman status, got %v", task3.Status)
	}
}

func TestReviewTask_HumanApproveAndReject(t *testing.T) {
	task, _ := NewReviewTask(104, ReviewPolicyPreReview)
	_ = task.ApplyMachineResult(40, []string{"suspicious"})

	// Invalid reviewer
	if err := task.ApproveByHuman(0, "ok"); err != ErrUnauthorizedReviewer {
		t.Fatalf("expected ErrUnauthorizedReviewer, got %v", err)
	}

	// Valid approve
	if err := task.ApproveByHuman(999, "looks good"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !task.IsApproved() || task.ReviewerID != 999 {
		t.Fatalf("expected task approved by 999, got %+v", task)
	}

	// Repeated action fails
	if err := task.RejectByHuman(999, "cannot reject approved"); err != ErrTaskAlreadyProcessed {
		t.Fatalf("expected ErrTaskAlreadyProcessed, got %v", err)
	}

	// Test Human Reject
	task2, _ := NewReviewTask(105, ReviewPolicyPreReview)
	_ = task2.ApplyMachineResult(40, []string{"suspicious"})
	if err := task2.RejectByHuman(888, "violates community rule"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !task2.IsRejected() || task2.ReviewerID != 888 {
		t.Fatalf("expected task rejected by 888, got %+v", task2)
	}
}

func TestReviewTask_Takedown(t *testing.T) {
	task, _ := NewReviewTask(106, ReviewPolicyPostReview)
	_ = task.ApplyMachineResult(0, nil) // Approved

	// Invalid reviewer
	if err := task.Takedown(0, "illegal"); err != ErrUnauthorizedReviewer {
		t.Fatalf("expected ErrUnauthorizedReviewer, got %v", err)
	}

	// Valid takedown
	if err := task.Takedown(777, "copyright infringement"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !task.IsTakedown() || task.ReviewerID != 777 {
		t.Fatalf("expected takedown state, got %+v", task)
	}

	// Repeated takedown fails
	if err := task.Takedown(777, "already takedown"); err != ErrTaskAlreadyProcessed {
		t.Fatalf("expected ErrTaskAlreadyProcessed, got %v", err)
	}
}
