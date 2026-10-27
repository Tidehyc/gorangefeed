package e2e_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainfeed "github.com/Tidehyc/gorangefeed/internal/domain/feed"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	httpfeed "github.com/Tidehyc/gorangefeed/internal/http/feed"
	httpvideo "github.com/Tidehyc/gorangefeed/internal/http/video"
	"github.com/gin-gonic/gin"
)

type feedResponseEnvelope struct {
	Data struct {
		Items      []*domainfeed.FeedItem `json:"items"`
		NextCursor int64                  `json:"next_cursor"`
		HasMore    bool                   `json:"has_more"`
	} `json:"data"`
}

type videoDetailEnvelope struct {
	Data *domainvideo.VideoDetail `json:"data"`
}

func publishTestVideo(t *testing.T, engine *gin.Engine, token, title, playURL string) *domainvideo.VideoDetail {
	t.Helper()
	w := httptest.NewRecorder()
	req := newJSONRequest(http.MethodPost, "/api/v1/videos", httpvideo.PublishVideoRequest{
		Title:       title,
		Description: "E2E Feed Test Video Description",
		PlayURL:     playURL,
		DurationMs:  15000,
	}, "192.168.1.100")
	req.Header.Set("Authorization", "Bearer "+token)
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to publish video %q: code=%d, body=%s", title, w.Code, w.Body.String())
	}

	var resp videoDetailEnvelope
	parseJSONResponse(t, w, &resp)
	if resp.Data == nil || resp.Data.Video == nil {
		t.Fatalf("expected video data in response, got nil")
	}
	return resp.Data
}

func TestM3_FeedLifecycle_EndToEnd(t *testing.T) {
	engine, _, cleanup := setupE2ETest(t)
	if cleanup != nil {
		defer cleanup()
	}

	suffix := time.Now().Format("0405_000")
	authorUsername := fmt.Sprintf("feed_author_%s", suffix)
	viewerUsername := fmt.Sprintf("feed_viewer_%s", suffix)
	testPass := "SecurePass123"

	// ==========================================
	// Phase 1: User Registration & Authentication
	// ==========================================
	authorID, authorToken := registerAndLogin(t, engine, authorUsername, testPass)
	viewerID, viewerToken := registerAndLogin(t, engine, viewerUsername, testPass)
	if authorID <= 0 || authorToken == "" {
		t.Fatalf("invalid author credentials: id=%d", authorID)
	}
	if viewerID <= 0 || viewerToken == "" {
		t.Fatalf("invalid viewer credentials: id=%d", viewerID)
	}

	// ==========================================
	// Phase 2: Video Publication & Timeline Hook
	// Publish 6 videos: A, B, C, D, E, F
	// ==========================================
	vA := publishTestVideo(t, engine, authorToken, "Video A", "/media/videos/a.mp4")
	vB := publishTestVideo(t, engine, authorToken, "Video B", "/media/videos/b.mp4")
	vC := publishTestVideo(t, engine, authorToken, "Video C", "/media/videos/c.mp4")
	vD := publishTestVideo(t, engine, authorToken, "Video D", "/media/videos/d.mp4")
	vE := publishTestVideo(t, engine, authorToken, "Video E", "/media/videos/e.mp4")
	vF := publishTestVideo(t, engine, authorToken, "Video F", "/media/videos/f.mp4")

	allVideos := []*domainvideo.VideoDetail{vA, vB, vC, vD, vE, vF}
	for i := 0; i < len(allVideos)-1; i++ {
		if allVideos[i].Video.ID >= allVideos[i+1].Video.ID {
			t.Fatalf("expected strictly increasing video IDs: %d < %d", allVideos[i].Video.ID, allVideos[i+1].Video.ID)
		}
	}

	// ==========================================
	// Phase 3: Global Feed First Page (FR-FEED-01)
	// Anonymous request: cursor = 0, limit = 3
	// Expected: [vF, vE, vD], next_cursor = vD.Video.ID, has_more = true
	// ==========================================
	var page1NextCursor int64
	{
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodGet, "/api/v1/feed/global?cursor=0&limit=3", nil, "192.168.1.101")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for feed page 1, got %d, body=%s", w.Code, w.Body.String())
		}

		var page1 feedResponseEnvelope
		parseJSONResponse(t, w, &page1)

		if len(page1.Data.Items) != 3 {
			t.Fatalf("expected 3 items in page 1, got %d", len(page1.Data.Items))
		}
		if page1.Data.Items[0].ID != vF.Video.ID || page1.Data.Items[1].ID != vE.Video.ID || page1.Data.Items[2].ID != vD.Video.ID {
			t.Fatalf("expected [F, E, D], got [%d, %d, %d]", page1.Data.Items[0].ID, page1.Data.Items[1].ID, page1.Data.Items[2].ID)
		}
		if !page1.Data.HasMore {
			t.Fatalf("expected has_more to be true for page 1")
		}
		if page1.Data.NextCursor != vD.Video.ID {
			t.Fatalf("expected next_cursor to be %d, got %d", vD.Video.ID, page1.Data.NextCursor)
		}
		page1NextCursor = page1.Data.NextCursor
	}

	// ==========================================
	// Phase 4: Cursor Pagination Without Duplicates or Omissions
	// Request page 2: cursor = page1NextCursor, limit = 3
	// Expected: [vC, vB, vA]
	// ==========================================
	{
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/feed/global?cursor=%d&limit=3", page1NextCursor), nil, "192.168.1.101")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for feed page 2, got %d, body=%s", w.Code, w.Body.String())
		}

		var page2 feedResponseEnvelope
		parseJSONResponse(t, w, &page2)

		if len(page2.Data.Items) != 3 {
			t.Fatalf("expected 3 items in page 2, got %d", len(page2.Data.Items))
		}
		if page2.Data.Items[0].ID != vC.Video.ID || page2.Data.Items[1].ID != vB.Video.ID || page2.Data.Items[2].ID != vA.Video.ID {
			t.Fatalf("expected [C, B, A], got [%d, %d, %d]", page2.Data.Items[0].ID, page2.Data.Items[1].ID, page2.Data.Items[2].ID)
		}
	}

	// ==========================================
	// Phase 5: Continuous Taken-down Videos Auto-Skip & Cursor Advance (FR-FEED-04)
	// Author deletes vE and vD. Feed scan should auto-skip them and return [vF, vC, vB]
	// ==========================================
	{
		// Delete vE
		wDelE := httptest.NewRecorder()
		reqDelE := newJSONRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d", vE.Video.ID), nil, "192.168.1.100")
		reqDelE.Header.Set("Authorization", "Bearer "+authorToken)
		engine.ServeHTTP(wDelE, reqDelE)
		if wDelE.Code != http.StatusOK {
			t.Fatalf("failed to delete video E: code=%d", wDelE.Code)
		}

		// Delete vD
		wDelD := httptest.NewRecorder()
		reqDelD := newJSONRequest(http.MethodDelete, fmt.Sprintf("/api/v1/videos/%d", vD.Video.ID), nil, "192.168.1.100")
		reqDelD.Header.Set("Authorization", "Bearer "+authorToken)
		engine.ServeHTTP(wDelD, reqDelD)
		if wDelD.Code != http.StatusOK {
			t.Fatalf("failed to delete video D: code=%d", wDelD.Code)
		}

		// Request fresh feed from top (bypass L1 with unique limit=4)
		w := httptest.NewRecorder()
		req := newJSONRequest(http.MethodGet, "/api/v1/feed/global?cursor=0&limit=4", nil, "192.168.1.102")
		engine.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 OK after taking down videos, got %d", w.Code)
		}

		var page feedResponseEnvelope
		parseJSONResponse(t, w, &page)

		// Expected active videos: F, C, B, A (E and D are skipped!)
		if len(page.Data.Items) != 4 {
			t.Fatalf("expected 4 active items, got %d", len(page.Data.Items))
		}
		for _, item := range page.Data.Items {
			if item.ID == vE.Video.ID || item.ID == vD.Video.ID {
				t.Fatalf("deleted video %d appeared in feed!", item.ID)
			}
		}
		if page.Data.Items[0].ID != vF.Video.ID || page.Data.Items[1].ID != vC.Video.ID || page.Data.Items[2].ID != vB.Video.ID || page.Data.Items[3].ID != vA.Video.ID {
			t.Fatalf("expected [F, C, B, A], got [%d, %d, %d, %d]", page.Data.Items[0].ID, page.Data.Items[1].ID, page.Data.Items[2].ID, page.Data.Items[3].ID)
		}
	}

	// ==========================================
	// Phase 6: Playback Impression & View Reporting Telemetry (FR-VID-08)
	// POST /api/v1/videos/:videoId/view drives view_count atomic increment
	// ==========================================
	{
		// 1. Initial view count on vF should be 0
		wDetailInit := httptest.NewRecorder()
		reqDetailInit := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d", vF.Video.ID), nil, "192.168.1.103")
		engine.ServeHTTP(wDetailInit, reqDetailInit)
		var detailInit videoDetailEnvelope
		parseJSONResponse(t, wDetailInit, &detailInit)
		if detailInit.Data.Stats.ViewCount != 0 {
			t.Fatalf("expected initial view count 0, got %d", detailInit.Data.Stats.ViewCount)
		}

		// 2. Report play view
		wView1 := httptest.NewRecorder()
		reqView1 := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/view", vF.Video.ID), httpfeed.RecordViewRequest{
			EventType:       "play",
			WatchDurationMs: 4000,
		}, "192.168.1.103")
		reqView1.Header.Set("Authorization", "Bearer "+viewerToken)
		engine.ServeHTTP(wView1, reqView1)
		if wView1.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for view report 1, got %d, body=%s", wView1.Code, wView1.Body.String())
		}

		// 3. Check updated view count on vF -> should be 1
		wDetailAfter1 := httptest.NewRecorder()
		reqDetailAfter1 := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d", vF.Video.ID), nil, "192.168.1.103")
		engine.ServeHTTP(wDetailAfter1, reqDetailAfter1)
		var detailAfter1 videoDetailEnvelope
		parseJSONResponse(t, wDetailAfter1, &detailAfter1)
		if detailAfter1.Data.Stats.ViewCount != 1 {
			t.Fatalf("expected view count 1 after first report, got %d", detailAfter1.Data.Stats.ViewCount)
		}

		// 4. Report complete view
		wView2 := httptest.NewRecorder()
		reqView2 := newJSONRequest(http.MethodPost, fmt.Sprintf("/api/v1/videos/%d/view", vF.Video.ID), httpfeed.RecordViewRequest{
			EventType:       "complete",
			WatchDurationMs: 15000,
		}, "192.168.1.103")
		reqView2.Header.Set("Authorization", "Bearer "+viewerToken)
		engine.ServeHTTP(wView2, reqView2)
		if wView2.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for view report 2, got %d", wView2.Code)
		}

		// 5. Check updated view count on vF -> should be 2
		wDetailAfter2 := httptest.NewRecorder()
		reqDetailAfter2 := newJSONRequest(http.MethodGet, fmt.Sprintf("/api/v1/videos/%d", vF.Video.ID), nil, "192.168.1.103")
		engine.ServeHTTP(wDetailAfter2, reqDetailAfter2)
		var detailAfter2 videoDetailEnvelope
		parseJSONResponse(t, wDetailAfter2, &detailAfter2)
		if detailAfter2.Data.Stats.ViewCount != 2 {
			t.Fatalf("expected view count 2 after second report, got %d", detailAfter2.Data.Stats.ViewCount)
		}
	}

	// ==========================================
	// Phase 7: Soft Auth Context Isolation
	// Verify anonymous request and authenticated request both succeed
	// ==========================================
	{
		// Anonymous request
		wAnon := httptest.NewRecorder()
		reqAnon := newJSONRequest(http.MethodGet, "/api/v1/feed/global?limit=2", nil, "192.168.1.104")
		engine.ServeHTTP(wAnon, reqAnon)
		if wAnon.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for anonymous request, got %d", wAnon.Code)
		}

		// Authenticated request
		wAuth := httptest.NewRecorder()
		reqAuth := newJSONRequest(http.MethodGet, "/api/v1/feed/global?limit=2", nil, "192.168.1.104")
		reqAuth.Header.Set("Authorization", "Bearer "+viewerToken)
		engine.ServeHTTP(wAuth, reqAuth)
		if wAuth.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for authenticated request, got %d", wAuth.Code)
		}
	}

	// ==========================================
	// Phase 8: End of Feed Handling
	// Query past the lowest possible video ID (cursor = 1) -> should return empty with has_more=false
	// ==========================================
	{
		wEnd := httptest.NewRecorder()
		reqEnd := newJSONRequest(http.MethodGet, "/api/v1/feed/global?cursor=1&limit=10", nil, "192.168.1.105")
		engine.ServeHTTP(wEnd, reqEnd)

		if wEnd.Code != http.StatusOK {
			t.Fatalf("expected 200 OK at end of feed, got %d", wEnd.Code)
		}

		var pageEnd feedResponseEnvelope
		parseJSONResponse(t, wEnd, &pageEnd)

		if len(pageEnd.Data.Items) != 0 {
			t.Fatalf("expected 0 items past oldest video, got %d", len(pageEnd.Data.Items))
		}
		if pageEnd.Data.HasMore {
			t.Fatalf("expected has_more to be false at end of feed")
		}
		if pageEnd.Data.NextCursor != 0 {
			t.Fatalf("expected next_cursor 0 at end of feed, got %d", pageEnd.Data.NextCursor)
		}
	}
}
