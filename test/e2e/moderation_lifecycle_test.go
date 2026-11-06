package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainmoderation "github.com/Tidehyc/gorangefeed/internal/domain/moderation"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpaccount "github.com/Tidehyc/gorangefeed/internal/http/account"
	"github.com/gin-gonic/gin"
)

type adminReviewListEnvelope struct {
	Code int `json:"code"`
	Data struct {
		Items      []*domainmoderation.ReviewTask `json:"items"`
		NextCursor int64                          `json:"next_cursor"`
		HasMore    bool                           `json:"has_more"`
	} `json:"data"`
}

type adminReviewActionEnvelope struct {
	Code int `json:"code"`
	Data struct {
		VideoID int64  `json:"video_id"`
		Action  string `json:"action"`
		Status  string `json:"status"`
	} `json:"data"`
}

type adminTakedownEnvelope struct {
	Code int `json:"code"`
	Data struct {
		VideoID      int64  `json:"video_id"`
		Status       string `json:"status"`
		IndexRevoked bool   `json:"index_revoked"`
	} `json:"data"`
}

type adminBanUserEnvelope struct {
	Code int `json:"code"`
	Data struct {
		UserID       int64  `json:"user_id"`
		Status       string `json:"status"`
		TokenRevoked bool   `json:"token_revoked"`
	} `json:"data"`
}

func TestM9_ModerationLifecycle_EndToEnd(t *testing.T) {
	ctxExt, cleanup := setupE2ETestExtended(t)
	if cleanup != nil {
		defer cleanup()
	}

	engine := ctxExt.Engine
	videoRepo := ctxExt.VideoRepo
	timelineRepo := ctxExt.TimelineRepo
	moderationSvc := ctxExt.ModerationSvc
	jwtMgr := ctxExt.JWTMgr

	suffix := time.Now().Format("0405_000")
	authorUsername := fmt.Sprintf("creator_mod_%s", suffix)
	adminUsername := fmt.Sprintf("admin_mod_%s", suffix)
	testPass := "SecurePass123"

	// 1. Register Author & Admin users. Roles are granted out-of-band, the same way an operator
	// would promote the first administrator.
	authorID, authorToken := registerAndLogin(t, engine, authorUsername, testPass)
	adminID, adminToken := registerAndLogin(t, engine, adminUsername, testPass)
	if err := ctxExt.DB.Model(&domainaccount.User{}).Where("id = ?", adminID).
		Update("role", domainaccount.RoleAdmin).Error; err != nil {
		t.Fatalf("failed to promote admin user: %v", err)
	}

	t.Logf("Author ID: %d, Admin ID: %d", authorID, adminID)

	// =========================================================================
	// Case 0: A regular authenticated user must not reach moderation endpoints
	// =========================================================================
	t.Run("NonAdmin_ForbiddenFromAdminEndpoints", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/reviews?status=2", nil)
		req.Header.Set("Authorization", "Bearer "+authorToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for a regular user hitting admin API, got %d: %s", w.Code, w.Body.String())
		}
	})

	// =========================================================================
	// Case 1: Machine inspection auto-rejects high-risk content & removes from timeline
	// =========================================================================
	t.Run("MachineAutoReject_HighRiskContent", func(t *testing.T) {
		video := &domainvideo.Video{
			AuthorID:    authorID,
			Title:       "独家线上赌博包赢秘籍",
			Description: "暴利彩票与赌博下注技巧",
			PlayURL:     "http://example.com/gambling.mp4",
			CoverURL:    "http://example.com/cover1.jpg",
			DurationMs:  15000,
			Status:      domainvideo.VideoStatusPublished,
		}
		stats := &domainvideo.VideoStats{VideoID: 0}
		if err := videoRepo.CreateWithStats(context.Background(), video, stats); err != nil {
			t.Fatalf("failed to create high-risk video: %v", err)
		}
		// Append to timeline initially
		_ = timelineRepo.Append(context.Background(), video.ID)

		// Submit for review
		task, err := moderationSvc.SubmitVideoForReview(
			context.Background(),
			video.ID,
			domainmoderation.ReviewPolicyPostReview,
			video.Title,
			video.Description,
		)
		if err != nil {
			t.Fatalf("unexpected error submitting for review: %v", err)
		}

		if task.Status != domainmoderation.ReviewStatusRejected {
			t.Fatalf("expected task status to be Rejected(4), got %d", task.Status)
		}
		if task.RiskScore <= 60 {
			t.Fatalf("expected risk score > 60, got %d", task.RiskScore)
		}

		// Verify video status is hidden in DB
		updatedVideo, err := videoRepo.FindByID(context.Background(), video.ID)
		if err != domainvideo.ErrVideoNotFound && updatedVideo != nil && updatedVideo.Status != domainvideo.VideoStatusHidden {
			t.Fatalf("expected video status to be hidden(2)")
		}

		// Verify video is removed from global timeline
		timelineIDs, _ := timelineRepo.ListByCursor(context.Background(), 0, 50)
		for _, tid := range timelineIDs {
			if tid == video.ID {
				t.Fatalf("expected video %d to be pruned from global timeline", video.ID)
			}
		}
	})

	// =========================================================================
	// Case 2: Clean content auto-approved under Post-Review policy
	// =========================================================================
	var cleanVideoID int64
	t.Run("MachineAutoApprove_CleanContent", func(t *testing.T) {
		video := &domainvideo.Video{
			AuthorID:    authorID,
			Title:       "可爱的橘猫在阳光下打呼噜",
			Description: "周末猫咪治愈时刻，温暖下午茶",
			PlayURL:     "http://example.com/cat.mp4",
			CoverURL:    "http://example.com/cat_cover.jpg",
			DurationMs:  20000,
			Status:      domainvideo.VideoStatusPublished,
		}
		stats := &domainvideo.VideoStats{VideoID: 0}
		if err := videoRepo.CreateWithStats(context.Background(), video, stats); err != nil {
			t.Fatalf("failed to create clean video: %v", err)
		}
		_ = timelineRepo.Append(context.Background(), video.ID)
		cleanVideoID = video.ID

		task, err := moderationSvc.SubmitVideoForReview(
			context.Background(),
			video.ID,
			domainmoderation.ReviewPolicyPostReview,
			video.Title,
			video.Description,
		)
		if err != nil {
			t.Fatalf("unexpected error submitting clean video: %v", err)
		}
		if task.Status != domainmoderation.ReviewStatusApproved {
			t.Fatalf("expected clean video status to be Approved(3), got %d", task.Status)
		}
		if task.RiskScore != 0 {
			t.Fatalf("expected 0 risk score, got %d", task.RiskScore)
		}
	})

	// =========================================================================
	// Case 3: Medium risk flagged -> Admin lists pending & manual approves
	// =========================================================================
	var flaggedVideoID int64
	t.Run("AdminReview_FlaggedVideoManualApprove", func(t *testing.T) {
		video := &domainvideo.Video{
			AuthorID:    authorID,
			Title:       "兼职刷单月入过万真相大揭秘",
			Description: "深度反诈科普：揭露兼职刷单私聊骗局",
			PlayURL:     "http://example.com/scam_expose.mp4",
			CoverURL:    "http://example.com/scam_cover.jpg",
			DurationMs:  30000,
			Status:      domainvideo.VideoStatusPublished,
		}
		stats := &domainvideo.VideoStats{VideoID: 0}
		_ = videoRepo.CreateWithStats(context.Background(), video, stats)
		flaggedVideoID = video.ID

		// Submit -> flagged as PendingHuman
		task, err := moderationSvc.SubmitVideoForReview(
			context.Background(),
			video.ID,
			domainmoderation.ReviewPolicyPreReview,
			video.Title,
			video.Description,
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if task.Status != domainmoderation.ReviewStatusPendingHuman {
			t.Fatalf("expected PendingHuman(2), got %d", task.Status)
		}

		// Admin lists pending tasks via GET /api/v1/admin/reviews?status=2
		req, _ := http.NewRequest(http.MethodGet, "/api/v1/admin/reviews?status=2&limit=20", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("failed to list pending reviews: %d %s", w.Code, w.Body.String())
		}
		var listResp adminReviewListEnvelope
		parseJSONResponse(t, w, &listResp)

		foundFlagged := false
		for _, item := range listResp.Data.Items {
			if item.VideoID == flaggedVideoID {
				foundFlagged = true
				break
			}
		}
		if !foundFlagged {
			t.Fatalf("expected flagged video %d to appear in admin pending list", flaggedVideoID)
		}

		// Admin manually approves video via POST /api/v1/admin/videos/:id/review
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"action":      "approve",
			"review_note": "verified as educational anti-fraud content",
		})
		revReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/videos/%d/review", flaggedVideoID), bytes.NewReader(bodyBytes))
		revReq.Header.Set("Authorization", "Bearer "+adminToken)
		revReq.Header.Set("Content-Type", "application/json")
		revW := httptest.NewRecorder()
		engine.ServeHTTP(revW, revReq)

		if revW.Code != http.StatusOK {
			t.Fatalf("failed to approve review task: %d %s", revW.Code, revW.Body.String())
		}
		var revResp adminReviewActionEnvelope
		parseJSONResponse(t, revW, &revResp)
		if revResp.Data.Status != "approved" {
			t.Fatalf("expected status approved, got %s", revResp.Data.Status)
		}
	})

	// =========================================================================
	// Case 4: Admin takedown of previously approved video & timeline revoked
	// =========================================================================
	t.Run("AdminTakedown_RevokesVideoAndIndex", func(t *testing.T) {
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"reason": "urgent copyright takedown requested by rights owner",
		})
		tdReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/videos/%d/takedown", cleanVideoID), bytes.NewReader(bodyBytes))
		tdReq.Header.Set("Authorization", "Bearer "+adminToken)
		tdReq.Header.Set("Content-Type", "application/json")
		tdW := httptest.NewRecorder()
		engine.ServeHTTP(tdW, tdReq)

		if tdW.Code != http.StatusOK {
			t.Fatalf("failed to takedown video: %d %s", tdW.Code, tdW.Body.String())
		}
		var tdResp adminTakedownEnvelope
		parseJSONResponse(t, tdW, &tdResp)
		if tdResp.Data.Status != "takedown" || !tdResp.Data.IndexRevoked {
			t.Fatalf("expected takedown status and index revoked")
		}

		// Assert timeline no longer has cleanVideoID
		timelineIDs, _ := timelineRepo.ListByCursor(context.Background(), 0, 50)
		for _, tid := range timelineIDs {
			if tid == cleanVideoID {
				t.Fatalf("expected video %d to be pruned from global timeline after takedown", cleanVideoID)
			}
		}
	})

	// =========================================================================
	// Case 5: Admin bans malicious user & verify token revocation / suspension
	// =========================================================================
	t.Run("AdminBanUser_SuspendsAccountAndAuditLog", func(t *testing.T) {
		maliciousUsername := fmt.Sprintf("spammer_%s", suffix)
		maliciousID, _ := registerAndLogin(t, engine, maliciousUsername, testPass)
		spammerAccess, spammerRefresh := loginTokens(t, engine, maliciousUsername, testPass)

		// Admin calls POST /api/v1/admin/users/:id/ban
		bodyBytes, _ := json.Marshal(map[string]interface{}{
			"reason": "severe community guideline violations: repeated spam and scam attempts",
		})
		banReq, _ := http.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/admin/users/%d/ban", maliciousID), bytes.NewReader(bodyBytes))
		banReq.Header.Set("Authorization", "Bearer "+adminToken)
		banReq.Header.Set("Content-Type", "application/json")
		banW := httptest.NewRecorder()
		engine.ServeHTTP(banW, banReq)

		if banW.Code != http.StatusOK {
			t.Fatalf("failed to ban user: %d %s", banW.Code, banW.Body.String())
		}
		var banResp adminBanUserEnvelope
		parseJSONResponse(t, banW, &banResp)
		if banResp.Data.Status != "suspended" {
			t.Fatalf("expected suspended status, got %s", banResp.Data.Status)
		}
		if !banResp.Data.TokenRevoked {
			t.Fatalf("expected ban response to report token_revoked=true")
		}

		// The ban must cut off credentials that were issued before it: a token that is still
		// cryptographically valid and not blacklisted must be rejected on the account level.
		meReq, _ := http.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
		meReq.Header.Set("Authorization", "Bearer "+spammerAccess)
		meW := httptest.NewRecorder()
		engine.ServeHTTP(meW, meReq)
		if meW.Code != http.StatusForbidden {
			t.Fatalf("expected 403 for a banned account's access token, got %d: %s", meW.Code, meW.Body.String())
		}

		// The refresh chain must be closed as well, otherwise the ban is renewable forever.
		refreshW := httptest.NewRecorder()
		engine.ServeHTTP(refreshW, newJSONRequest(http.MethodPost, "/api/v1/auth/refresh", map[string]string{
			"refresh_token": spammerRefresh,
		}, fmt.Sprintf("192.168.41.%d", time.Now().Nanosecond()%200+10)))
		if refreshW.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 refreshing a banned account, got %d: %s", refreshW.Code, refreshW.Body.String())
		}

		// Re-authentication is refused outright.
		reloginW := httptest.NewRecorder()
		engine.ServeHTTP(reloginW, newJSONRequest(http.MethodPost, "/api/v1/auth/login", httpaccount.LoginRequest{
			Username: maliciousUsername,
			Password: testPass,
		}, fmt.Sprintf("192.168.42.%d", time.Now().Nanosecond()%200+10)))
		if reloginW.Code != http.StatusForbidden {
			t.Fatalf("expected 403 logging in a banned account, got %d: %s", reloginW.Code, reloginW.Body.String())
		}

		// Verify audit logs were created
		logs, err := moderationSvc.ListAuditLogs(context.Background(), domainmoderation.TargetTypeUser, maliciousID, 10)
		if err != nil || len(logs) == 0 {
			t.Fatalf("expected audit log for banned user, got %v", logs)
		}
		if logs[0].OperatorID != adminID || logs[0].Action != domainmoderation.ActionBanUser {
			t.Fatalf("unexpected audit log: %+v", logs[0])
		}
	})

	_ = jwtMgr
}

// loginTokens authenticates an existing account and returns both credential types, so ban
// enforcement can be asserted against each one independently.
func loginTokens(t *testing.T, engine *gin.Engine, username, password string) (string, string) {
	t.Helper()
	ip := fmt.Sprintf("192.168.43.%d", time.Now().Nanosecond()%200+10)

	w := httptest.NewRecorder()
	engine.ServeHTTP(w, newJSONRequest(http.MethodPost, "/api/v1/auth/login", httpaccount.LoginRequest{
		Username: username,
		Password: password,
	}, ip))
	if w.Code != http.StatusOK {
		t.Fatalf("failed to login user %s: code=%d, body=%s", username, w.Code, w.Body.String())
	}

	var resp struct {
		Data httpaccount.LoginResponse `json:"data"`
	}
	parseJSONResponse(t, w, &resp)
	return resp.Data.Tokens.AccessToken, resp.Data.Tokens.RefreshToken
}
