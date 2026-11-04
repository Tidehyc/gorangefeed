package asset_test

import (
	"context"
	"errors"
	"testing"
	"time"

	domainasset "github.com/Tidehyc/gorangefeed/internal/domain/asset"
	domainoutbox "github.com/Tidehyc/gorangefeed/internal/domain/outbox"
	domainvideo "github.com/Tidehyc/gorangefeed/internal/domain/video"
	serviceasset "github.com/Tidehyc/gorangefeed/internal/service/asset"
)

// --- Mock Implementations ---

type mockFavoriteRepo struct {
	defaultFolder  *domainasset.FavoriteFolder
	folders        map[int64]*domainasset.FavoriteFolder
	items          map[int64][]*domainasset.FavoriteItem // folderID -> items
	savedOutbox    []*domainoutbox.Event
	addItemFunc    func(ctx context.Context, item *domainasset.FavoriteItem, outboxEvent *domainoutbox.Event) (bool, error)
	removeItemFunc func(ctx context.Context, favoriteID, videoID int64, outboxEvent *domainoutbox.Event) (bool, error)
}

func newMockFavoriteRepo() *mockFavoriteRepo {
	return &mockFavoriteRepo{
		folders:     make(map[int64]*domainasset.FavoriteFolder),
		items:       make(map[int64][]*domainasset.FavoriteItem),
		savedOutbox: make([]*domainoutbox.Event, 0),
	}
}

func (m *mockFavoriteRepo) GetOrCreateDefaultFolder(ctx context.Context, userID int64) (*domainasset.FavoriteFolder, error) {
	if m.defaultFolder != nil {
		return m.defaultFolder, nil
	}
	f := &domainasset.FavoriteFolder{
		ID:        1,
		UserID:    userID,
		Name:      domainasset.DefaultFolderName,
		IsDefault: true,
		IsPublic:  true,
	}
	m.defaultFolder = f
	m.folders[1] = f
	return f, nil
}

func (m *mockFavoriteRepo) CreateFolder(ctx context.Context, folder *domainasset.FavoriteFolder) error {
	folder.ID = int64(len(m.folders) + 10)
	folder.CreatedAt = time.Now()
	folder.UpdatedAt = time.Now()
	m.folders[folder.ID] = folder
	return nil
}

func (m *mockFavoriteRepo) FindFolderByID(ctx context.Context, folderID int64) (*domainasset.FavoriteFolder, error) {
	if f, ok := m.folders[folderID]; ok {
		return f, nil
	}
	return nil, domainasset.ErrFolderNotFound
}

func (m *mockFavoriteRepo) ListFolders(ctx context.Context, userID int64) ([]*domainasset.FavoriteFolder, error) {
	var list []*domainasset.FavoriteFolder
	for _, f := range m.folders {
		if f.UserID == userID {
			list = append(list, f)
		}
	}
	return list, nil
}

func (m *mockFavoriteRepo) DeleteFolder(ctx context.Context, userID, folderID int64) error {
	f, ok := m.folders[folderID]
	if !ok {
		return domainasset.ErrFolderNotFound
	}
	if f.UserID != userID {
		return domainasset.ErrFolderAccessDenied
	}
	if f.IsDefault {
		return domainasset.ErrCannotDeleteDefaultFolder
	}
	delete(m.folders, folderID)
	delete(m.items, folderID)
	return nil
}

func (m *mockFavoriteRepo) AddItem(ctx context.Context, item *domainasset.FavoriteItem, outboxEvent *domainoutbox.Event) (bool, error) {
	if m.addItemFunc != nil {
		return m.addItemFunc(ctx, item, outboxEvent)
	}
	if outboxEvent != nil {
		m.savedOutbox = append(m.savedOutbox, outboxEvent)
	}
	item.ID = 100
	m.items[item.FavoriteID] = append(m.items[item.FavoriteID], item)
	return true, nil
}

func (m *mockFavoriteRepo) RemoveItem(ctx context.Context, favoriteID, videoID int64, outboxEvent *domainoutbox.Event) (bool, error) {
	if m.removeItemFunc != nil {
		return m.removeItemFunc(ctx, favoriteID, videoID, outboxEvent)
	}
	if outboxEvent != nil {
		m.savedOutbox = append(m.savedOutbox, outboxEvent)
	}
	return true, nil
}

func (m *mockFavoriteRepo) IsFavorited(ctx context.Context, userID, videoID int64) (bool, error) {
	for _, items := range m.items {
		for _, item := range items {
			if item.UserID == userID && item.VideoID == videoID {
				return true, nil
			}
		}
	}
	return false, nil
}

func (m *mockFavoriteRepo) BatchIsFavorited(ctx context.Context, userID int64, videoIDs []int64) (map[int64]bool, error) {
	res := make(map[int64]bool, len(videoIDs))
	for _, vid := range videoIDs {
		res[vid] = false
	}
	for _, items := range m.items {
		for _, item := range items {
			if item.UserID == userID {
				res[item.VideoID] = true
			}
		}
	}
	return res, nil
}

func (m *mockFavoriteRepo) ListItems(ctx context.Context, favoriteID int64, cursor int64, limit int) ([]*domainasset.FavoriteItem, int64, bool, error) {
	items := m.items[favoriteID]
	return items, 0, false, nil
}

type mockHistoryRepo struct {
	records      map[int64]*domainasset.WatchHistory // videoID -> record
	recordErr    error
	getProgress  func(ctx context.Context, userID, videoID int64) (*domainasset.WatchHistory, error)
	clearHistory func(ctx context.Context, userID int64) error
}

func newMockHistoryRepo() *mockHistoryRepo {
	return &mockHistoryRepo{
		records: make(map[int64]*domainasset.WatchHistory),
	}
}

func (m *mockHistoryRepo) RecordProgress(ctx context.Context, history *domainasset.WatchHistory) error {
	if m.recordErr != nil {
		return m.recordErr
	}
	m.records[history.VideoID] = history
	return nil
}

func (m *mockHistoryRepo) RecordProgressSync(ctx context.Context, history *domainasset.WatchHistory) error {
	return m.RecordProgress(ctx, history)
}

func (m *mockHistoryRepo) GetProgress(ctx context.Context, userID, videoID int64) (*domainasset.WatchHistory, error) {
	if m.getProgress != nil {
		return m.getProgress(ctx, userID, videoID)
	}
	return m.records[videoID], nil
}

func (m *mockHistoryRepo) ListHistory(ctx context.Context, userID int64, limit int) ([]*domainasset.WatchHistory, error) {
	var list []*domainasset.WatchHistory
	for _, h := range m.records {
		list = append(list, h)
	}
	return list, nil
}

func (m *mockHistoryRepo) ClearHistory(ctx context.Context, userID int64) error {
	if m.clearHistory != nil {
		return m.clearHistory(ctx, userID)
	}
	m.records = make(map[int64]*domainasset.WatchHistory)
	return nil
}

type mockVideoRepo struct {
	videos map[int64]*domainvideo.Video
}

func newMockVideoRepo() *mockVideoRepo {
	return &mockVideoRepo{
		videos: make(map[int64]*domainvideo.Video),
	}
}

func (m *mockVideoRepo) FindByID(ctx context.Context, id int64) (*domainvideo.Video, error) {
	if v, ok := m.videos[id]; ok {
		return v, nil
	}
	return nil, domainvideo.ErrVideoNotFound
}

func (m *mockVideoRepo) CreateWithStats(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats) error {
	return nil
}

func (m *mockVideoRepo) CreateWithStatsAndOutbox(ctx context.Context, video *domainvideo.Video, stats *domainvideo.VideoStats, outboxEvent *domainoutbox.Event) error {
	return nil
}

func (m *mockVideoRepo) FindDetailByID(ctx context.Context, id int64) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	v, err := m.FindByID(ctx, id)
	return v, &domainvideo.VideoStats{VideoID: id}, err
}

func (m *mockVideoRepo) FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, key string) (*domainvideo.Video, *domainvideo.VideoStats, error) {
	return nil, nil, nil
}

func (m *mockVideoRepo) ListByAuthorCursor(ctx context.Context, authorID int64, cursor int64, limit int) ([]*domainvideo.Video, []*domainvideo.VideoStats, int64, bool, error) {
	return nil, nil, 0, false, nil
}

func (m *mockVideoRepo) UpdateStatus(ctx context.Context, id int64, authorID int64, status domainvideo.VideoStatus) error {
	if v, ok := m.videos[id]; ok {
		if v.AuthorID != authorID {
			return domainvideo.ErrVideoForbidden
		}
		v.Status = status
		return nil
	}
	return domainvideo.ErrVideoNotFound
}

func (m *mockVideoRepo) UpdatePublishSeq(ctx context.Context, videoID int64, publishSeq int64) error {
	if v, ok := m.videos[videoID]; ok {
		v.PublishSeq = publishSeq
		return nil
	}
	return domainvideo.ErrVideoNotFound
}

// --- Tests ---

func TestService_Initialization(t *testing.T) {
	favRepo := newMockFavoriteRepo()
	histRepo := newMockHistoryRepo()
	vidRepo := newMockVideoRepo()

	_, err := serviceasset.NewService(nil, histRepo, vidRepo)
	if err == nil {
		t.Fatalf("expected error on nil favorite repo")
	}

	_, err = serviceasset.NewService(favRepo, nil, vidRepo)
	if err == nil {
		t.Fatalf("expected error on nil history repo")
	}

	_, err = serviceasset.NewService(favRepo, histRepo, nil)
	if err == nil {
		t.Fatalf("expected error on nil video repo")
	}

	svc, err := serviceasset.NewService(favRepo, histRepo, vidRepo)
	if err != nil || svc == nil {
		t.Fatalf("expected successful construction, got %v", err)
	}
}

func TestService_FavoriteVideo_DefaultAndCustom(t *testing.T) {
	favRepo := newMockFavoriteRepo()
	histRepo := newMockHistoryRepo()
	vidRepo := newMockVideoRepo()
	svc, _ := serviceasset.NewService(favRepo, histRepo, vidRepo)
	ctx := context.Background()

	authorID := int64(10)
	userID := int64(20)
	videoID := int64(101)

	vidRepo.videos[videoID] = &domainvideo.Video{
		ID:       videoID,
		AuthorID: authorID,
		Status:   domainvideo.VideoStatusPublished,
	}

	// 1. Favorite to default folder (folderID = nil)
	item, inserted, err := svc.FavoriteVideo(ctx, userID, videoID, nil)
	if err != nil {
		t.Fatalf("failed to favorite to default folder: %v", err)
	}
	if !inserted {
		t.Fatalf("expected inserted to be true")
	}
	if item.FavoriteID != 1 {
		t.Fatalf("expected default folder ID 1, got %d", item.FavoriteID)
	}
	if len(favRepo.savedOutbox) != 1 {
		t.Fatalf("expected 1 outbox event generated, got %d", len(favRepo.savedOutbox))
	}
	if favRepo.savedOutbox[0].EventType != domainoutbox.EventTypeInteractionFavorited {
		t.Fatalf("expected event type interaction.favorited, got %s", favRepo.savedOutbox[0].EventType)
	}

	// 2. Create custom folder and favorite to custom folder
	customFolder, err := svc.CreateFolder(ctx, userID, "Special Clips", "My favorite clips", true)
	if err != nil {
		t.Fatalf("failed to create custom folder: %v", err)
	}

	item2, inserted2, err := svc.FavoriteVideo(ctx, userID, videoID, &customFolder.ID)
	if err != nil {
		t.Fatalf("failed to favorite to custom folder: %v", err)
	}
	if !inserted2 {
		t.Fatalf("expected inserted2 to be true")
	}
	if item2.FavoriteID != customFolder.ID {
		t.Fatalf("expected custom folder ID %d, got %d", customFolder.ID, item2.FavoriteID)
	}
	if len(favRepo.savedOutbox) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(favRepo.savedOutbox))
	}
}

func TestService_FavoriteVideo_ValidationsAndErrors(t *testing.T) {
	favRepo := newMockFavoriteRepo()
	histRepo := newMockHistoryRepo()
	vidRepo := newMockVideoRepo()
	svc, _ := serviceasset.NewService(favRepo, histRepo, vidRepo)
	ctx := context.Background()

	// 1. Invalid userID / videoID
	_, _, err := svc.FavoriteVideo(ctx, 0, 100, nil)
	if err != domainasset.ErrInvalidUserID {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
	_, _, err = svc.FavoriteVideo(ctx, 100, 0, nil)
	if err != domainasset.ErrInvalidVideoID {
		t.Fatalf("expected ErrInvalidVideoID, got %v", err)
	}

	// 2. Video not found
	_, _, err = svc.FavoriteVideo(ctx, 100, 9999, nil)
	if !errors.Is(err, domainvideo.ErrVideoNotFound) {
		t.Fatalf("expected ErrVideoNotFound, got %v", err)
	}

	// 3. Hidden video
	vidRepo.videos[500] = &domainvideo.Video{
		ID:     500,
		Status: domainvideo.VideoStatusHidden,
	}
	_, _, err = svc.FavoriteVideo(ctx, 100, 500, nil)
	if !errors.Is(err, domainvideo.ErrVideoNotFound) {
		t.Fatalf("expected ErrVideoNotFound for hidden video, got %v", err)
	}

	// 4. Custom folder owned by another user
	vidRepo.videos[600] = &domainvideo.Video{
		ID:     600,
		Status: domainvideo.VideoStatusPublished,
	}
	favRepo.folders[777] = &domainasset.FavoriteFolder{
		ID:     777,
		UserID: 200, // owned by user 200
		Name:   "Someone else's folder",
	}
	targetFolderID := int64(777)
	_, _, err = svc.FavoriteVideo(ctx, 100, 600, &targetFolderID)
	if err != domainasset.ErrFolderAccessDenied {
		t.Fatalf("expected ErrFolderAccessDenied, got %v", err)
	}
}

func TestService_UnfavoriteVideo(t *testing.T) {
	favRepo := newMockFavoriteRepo()
	histRepo := newMockHistoryRepo()
	vidRepo := newMockVideoRepo()
	svc, _ := serviceasset.NewService(favRepo, histRepo, vidRepo)
	ctx := context.Background()

	userID := int64(10)
	videoID := int64(200)

	vidRepo.videos[videoID] = &domainvideo.Video{
		ID:       videoID,
		AuthorID: 99,
		Status:   domainvideo.VideoStatusPublished,
	}

	// Unfavorite from default folder
	removed, err := svc.UnfavoriteVideo(ctx, userID, videoID, nil)
	if err != nil {
		t.Fatalf("failed to unfavorite: %v", err)
	}
	if !removed {
		t.Fatalf("expected removed to be true")
	}
	if len(favRepo.savedOutbox) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(favRepo.savedOutbox))
	}
	if favRepo.savedOutbox[0].EventType != domainoutbox.EventTypeInteractionUnfavorited {
		t.Fatalf("expected interaction.unfavorited, got %s", favRepo.savedOutbox[0].EventType)
	}

	// Unfavorite video not found
	_, err = svc.UnfavoriteVideo(ctx, userID, 99999, nil)
	if !errors.Is(err, domainvideo.ErrVideoNotFound) {
		t.Fatalf("expected ErrVideoNotFound, got %v", err)
	}
}

func TestService_FolderManagementAndPrivacy(t *testing.T) {
	favRepo := newMockFavoriteRepo()
	histRepo := newMockHistoryRepo()
	vidRepo := newMockVideoRepo()
	svc, _ := serviceasset.NewService(favRepo, histRepo, vidRepo)
	ctx := context.Background()

	userID := int64(50)

	// 1. Create custom folder
	folder, err := svc.CreateFolder(ctx, userID, "Private Secrets", "Secret clips", false)
	if err != nil {
		t.Fatalf("failed to create private folder: %v", err)
	}
	if folder.IsPublic {
		t.Fatalf("expected folder to be private")
	}

	// 2. ListFolders ensures default folder is initialized
	folders, err := svc.ListFolders(ctx, userID)
	if err != nil {
		t.Fatalf("failed to list folders: %v", err)
	}
	if len(folders) != 2 { // default folder + custom folder
		t.Fatalf("expected 2 folders, got %d", len(folders))
	}

	// 3. Privacy check on ListFolderItems: owner can view
	_, _, _, err = svc.ListFolderItems(ctx, userID, folder.ID, 0, 10)
	if err != nil {
		t.Fatalf("owner should be allowed to view private folder items: %v", err)
	}

	// Other user cannot view private folder items
	otherUser := int64(999)
	_, _, _, err = svc.ListFolderItems(ctx, otherUser, folder.ID, 0, 10)
	if err != domainasset.ErrFolderAccessDenied {
		t.Fatalf("expected ErrFolderAccessDenied for other user, got %v", err)
	}

	// 4. Delete folder
	if err := svc.DeleteFolder(ctx, userID, folder.ID); err != nil {
		t.Fatalf("failed to delete folder: %v", err)
	}

	// Default folder cannot be deleted
	defaultFolder, _ := favRepo.GetOrCreateDefaultFolder(ctx, userID)
	if err := svc.DeleteFolder(ctx, userID, defaultFolder.ID); err != domainasset.ErrCannotDeleteDefaultFolder {
		t.Fatalf("expected ErrCannotDeleteDefaultFolder, got %v", err)
	}
}

func TestService_WatchHistoryAndBreakpointResume(t *testing.T) {
	favRepo := newMockFavoriteRepo()
	histRepo := newMockHistoryRepo()
	vidRepo := newMockVideoRepo()
	svc, _ := serviceasset.NewService(favRepo, histRepo, vidRepo)
	ctx := context.Background()

	userID := int64(100)
	videoID := int64(888)

	// 1. Invalid inputs
	_, err := svc.RecordWatchProgress(ctx, 0, videoID, 100, 200)
	if err != domainasset.ErrInvalidUserID {
		t.Fatalf("expected ErrInvalidUserID, got %v", err)
	}
	_, err = svc.RecordWatchProgress(ctx, userID, 0, 100, 200)
	if err != domainasset.ErrInvalidVideoID {
		t.Fatalf("expected ErrInvalidVideoID, got %v", err)
	}
	_, err = svc.RecordWatchProgress(ctx, userID, videoID, -5, 200)
	if err != domainasset.ErrInvalidPlaybackPosition {
		t.Fatalf("expected ErrInvalidPlaybackPosition, got %v", err)
	}
	_, err = svc.RecordWatchProgress(ctx, userID, videoID, 100, -10)
	if err != domainasset.ErrInvalidDuration {
		t.Fatalf("expected ErrInvalidDuration, got %v", err)
	}

	// 2. Record 50% progress
	hist, err := svc.RecordWatchProgress(ctx, userID, videoID, 30000, 60000)
	if err != nil {
		t.Fatalf("failed to record progress: %v", err)
	}
	if hist.IsFinished {
		t.Fatalf("expected not finished at 50%%")
	}

	// 3. Breakpoint resumption lookup
	got, err := svc.GetWatchProgress(ctx, userID, videoID)
	if err != nil {
		t.Fatalf("failed to get watch progress: %v", err)
	}
	if got.LastPositionMs != 30000 {
		t.Fatalf("expected last position 30000, got %d", got.LastPositionMs)
	}

	// 4. Record 96% progress -> triggers IsFinished
	hist2, err := svc.RecordWatchProgressSync(ctx, userID, videoID, 58000, 60000)
	if err != nil {
		t.Fatalf("failed to record progress sync: %v", err)
	}
	if !hist2.IsFinished {
		t.Fatalf("expected finished when reaching 96%% completion")
	}

	// 5. List history
	list, err := svc.ListWatchHistory(ctx, userID, 10)
	if err != nil {
		t.Fatalf("failed to list watch history: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(list))
	}

	// 6. Clear history
	if err := svc.ClearWatchHistory(ctx, userID); err != nil {
		t.Fatalf("failed to clear watch history: %v", err)
	}
	if len(histRepo.records) != 0 {
		t.Fatalf("expected 0 records after clear, got %d", len(histRepo.records))
	}
}

func TestService_FavoriteStatusQueries(t *testing.T) {
	favRepo := newMockFavoriteRepo()
	histRepo := newMockHistoryRepo()
	vidRepo := newMockVideoRepo()
	svc, _ := serviceasset.NewService(favRepo, histRepo, vidRepo)
	ctx := context.Background()

	userID := int64(1)
	favRepo.items[1] = []*domainasset.FavoriteItem{
		{FavoriteID: 1, VideoID: 10, UserID: userID},
		{FavoriteID: 1, VideoID: 20, UserID: userID},
	}

	isFav10, _ := svc.IsFavorited(ctx, userID, 10)
	isFav30, _ := svc.IsFavorited(ctx, userID, 30)
	if !isFav10 || isFav30 {
		t.Fatalf("expected 10 to be favorited and 30 not favorited")
	}

	batchMap, err := svc.BatchIsFavorited(ctx, userID, []int64{10, 20, 30})
	if err != nil {
		t.Fatalf("batch check error: %v", err)
	}
	if !batchMap[10] || !batchMap[20] || batchMap[30] {
		t.Fatalf("unexpected batch map: %+v", batchMap)
	}
}
