package video_test

import (
	"context"
	"testing"
	"time"

	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	servicevideo "github.com/Tidehyc/gorangefeed/internal/service/video"
)

// mockVideoRepo implements domainvideo.Repository for service unit testing.
type mockVideoRepo struct {
	createWithStatsFn               func(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error
	findByIDFn                      func(ctx context.Context, id int64) (*domainvideo.Video, error)
	findDetailByIDFn                func(ctx context.Context, id int64) (*domainvideo.Video, *domainvideo.VideoStats, error)
	findByAuthorAndIdempotencyKeyFn func(ctx context.Context, authorID int64, key string) (*domainvideo.Video, *domainvideo.VideoStats, error)
	listByAuthorCursorFn            func(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.Video, []*domainvideo.VideoStats, int64, bool, error)
	updateStatusFn                  func(ctx context.Context, id int64, authorID int64, status domainvideo.VideoStatus) error
}

func (m *mockVideoRepo) CreateWithStats(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error {
	if m.createWithStatsFn != nil {
		return m.createWithStatsFn(ctx, video, stats)
	}
	video.ID = 100
	return nil
}

func (m *mockVideoRepo) FindByID(ctx context.Context, id int64) (*domainvideo.Video, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return nil, domainvideo.ErrVideoNotFound
}

func (m *mockVideoRepo) FindDetailByID(ctx context.Context, id int64) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	if m.findDetailByIDFn != nil {
		return m.findDetailByIDFn(ctx, id)
	}
	return nil, nil, domainvideo.ErrVideoNotFound
}

func (m *mockVideoRepo) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	if m.findByAuthorAndIdempotencyKeyFn != nil {
		return m.findByAuthorAndIdempotencyKeyFn(ctx, authorID, key)
	}
	return nil, nil, domainvideo.ErrVideoNotFound
}

func (m *mockVideoRepo) ListByAuthorCursor(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.Video, []*domainvideo.VideoStats, int64, bool, error) {
	if m.listByAuthorCursorFn != nil {
		return m.listByAuthorCursorFn(ctx, authorID, cursor, limit)
	}
	return []*domainvideo.Video{}, []*domainvideo.VideoStats{}, 0, false, nil
}

func (m *mockVideoRepo) UpdateStatus(ctx context.Context, id int64, authorID int64, status domainvideo.VideoStatus) error {
	if m.updateStatusFn != nil {
		return m.updateStatusFn(ctx, id, authorID, status)
	}
	return nil
}

// mockAccountRepo implements domainaccount.Repository for service unit testing.
type mockAccountRepo struct {
	findByIDFn func(ctx context.Context, id int64) (*domainaccount.User, error)
}

func (m *mockAccountRepo) Create(ctx context.Context, user *domainaccount.User) error { return nil }
func (m *mockAccountRepo) FindByID(ctx context.Context, id int64) (*domainaccount.User, error) {
	if m.findByIDFn != nil {
		return m.findByIDFn(ctx, id)
	}
	return &domainaccount.User{
		ID:        id,
		Username:  "author_tester",
		Nickname:  "Tester Nick",
		AvatarURL: "http://example.com/avatar.jpg",
	}, nil
}
func (m *mockAccountRepo) FindByUsername(ctx context.Context, username string) (*domainaccount.User, error) {
	return nil, nil
}
func (m *mockAccountRepo) UpdateProfile(ctx context.Context, id int64, nickname, bio string) error {
	return nil
}
func (m *mockAccountRepo) UpdateAvatar(ctx context.Context, id int64, avatarURL string) error {
	return nil
}

func TestService_PublishSuccess(t *testing.T) {
	repo := &mockVideoRepo{
		createWithStatsFn: func(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error {
			video.ID = 123
			video.PublishedAt = time.Now()
			stats.VideoID = 123
			return nil
		},
	}
	accountRepo := &mockAccountRepo{}
	svc := servicevideo.NewService(repo, accountRepo)

	detail, err := svc.Publish(context.Background(), 1, "Awesome Video", "Desc", "http://example.com/play.mp4", "http://example.com/cover.jpg", 30000, "key123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if detail.Video.ID != 123 {
		t.Fatalf("expected video ID 123, got %d", detail.Video.ID)
	}
	if detail.Author.Username != "author_tester" {
		t.Fatalf("expected author username 'author_tester', got %q", detail.Author.Username)
	}
}

func TestService_PublishValidationErrors(t *testing.T) {
	svc := servicevideo.NewService(&mockVideoRepo{}, &mockAccountRepo{})
	ctx := context.Background()

	// Missing title
	_, err := svc.Publish(ctx, 1, "", "Desc", "http://example.com/play.mp4", "", 10, "")
	if err == nil {
		t.Fatal("expected error on empty title, got nil")
	}

	// Missing play URL
	_, err = svc.Publish(ctx, 1, "Title", "Desc", "", "", 10, "")
	if err == nil {
		t.Fatal("expected error on empty playURL, got nil")
	}

	// Invalid author ID
	_, err = svc.Publish(ctx, 0, "Title", "Desc", "http://example.com/play.mp4", "", 10, "")
	if err == nil {
		t.Fatal("expected error on invalid author ID, got nil")
	}
}

func TestService_PublishIdempotentReplay(t *testing.T) {
	existingVideo := &domainvideo.Video{
		ID:       888,
		AuthorID: 1,
		Title:    "Already Created Video",
		PlayURL:  "http://example.com/play.mp4",
	}
	existingStats := &domainvideo.VideoStats{
		VideoID:   888,
		LikeCount: 50,
	}

	repo := &mockVideoRepo{
		findByAuthorAndIdempotencyKeyFn: func(ctx context.Context, authorID int64, key string) (*domainvideo.Video, *domainvideo.VideoStats, error) {
			if key == "idem_key_hit" {
				return existingVideo, existingStats, nil
			}
			return nil, nil, domainvideo.ErrVideoNotFound
		},
		createWithStatsFn: func(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error {
			t.Fatal("CreateWithStats should not be called when idempotency key hits existing record")
			return nil
		},
	}
	svc := servicevideo.NewService(repo, &mockAccountRepo{})

	detail, err := svc.Publish(context.Background(), 1, "New Title", "Desc", "http://example.com/play.mp4", "", 10, "idem_key_hit")
	if err != nil {
		t.Fatalf("expected no error on idempotent replay, got %v", err)
	}
	if detail.Video.ID != 888 {
		t.Fatalf("expected replayed video ID 888, got %d", detail.Video.ID)
	}
	if detail.Stats.LikeCount != 50 {
		t.Fatalf("expected stats like count 50, got %d", detail.Stats.LikeCount)
	}
}

func TestService_GetDetail(t *testing.T) {
	repo := &mockVideoRepo{
		findDetailByIDFn: func(ctx context.Context, id int64) (*domainvideo.Video, *domainvideo.VideoStats, error) {
			if id == 42 {
				return &domainvideo.Video{
					ID:       42,
					AuthorID: 1,
					Title:    "Detail 42",
					PlayURL:  "http://example.com/42.mp4",
				}, &domainvideo.VideoStats{VideoID: 42, ViewCount: 200}, nil
			}
			return nil, nil, domainvideo.ErrVideoNotFound
		},
	}
	svc := servicevideo.NewService(repo, &mockAccountRepo{})

	// Success
	detail, err := svc.GetDetail(context.Background(), 42, 0)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if detail.Video.ID != 42 || detail.Stats.ViewCount != 200 {
		t.Fatalf("unexpected detail values")
	}

	// Not found
	_, err = svc.GetDetail(context.Background(), 999, 0)
	if err == nil {
		t.Fatal("expected error on missing video, got nil")
	}
}

func TestService_ListAuthorVideos(t *testing.T) {
	repo := &mockVideoRepo{
		listByAuthorCursorFn: func(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.Video, []*domainvideo.VideoStats, int64, bool, error) {
			v1 := &domainvideo.Video{ID: 10, AuthorID: authorID, Title: "Video 10"}
			v2 := &domainvideo.Video{ID: 9, AuthorID: authorID, Title: "Video 9"}
			s1 := &domainvideo.VideoStats{VideoID: 10}
			s2 := &domainvideo.VideoStats{VideoID: 9}
			return []*domainvideo.Video{v1, v2}, []*domainvideo.VideoStats{s1, s2}, 9, true, nil
		},
	}
	svc := servicevideo.NewService(repo, &mockAccountRepo{})

	details, nextCursor, hasMore, err := svc.ListAuthorVideos(context.Background(), 1, 0, 10)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(details) != 2 || nextCursor != 9 || !hasMore {
		t.Fatalf("unexpected pagination results: len=%d, nextCursor=%d, hasMore=%v", len(details), nextCursor, hasMore)
	}
}

func TestService_DeleteVideo(t *testing.T) {
	repo := &mockVideoRepo{
		updateStatusFn: func(ctx context.Context, id int64, authorID int64, status domainvideo.VideoStatus) error {
			if id == 100 && authorID == 1 {
				return nil
			}
			if id == 100 && authorID != 1 {
				return domainvideo.ErrVideoForbidden
			}
			return domainvideo.ErrVideoNotFound
		},
	}
	svc := servicevideo.NewService(repo, &mockAccountRepo{})
	ctx := context.Background()

	// Author delete -> success
	if err := svc.DeleteVideo(ctx, 100, 1); err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// Non-author delete -> forbidden
	err := svc.DeleteVideo(ctx, 100, 2)
	if err == nil {
		t.Fatal("expected forbidden error, got nil")
	}

	// Not found delete -> not found
	err = svc.DeleteVideo(ctx, 999, 1)
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
}
