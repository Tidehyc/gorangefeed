package upload_test

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	domainupload "github.com/Tidehyc/gorangefeed/internal/domain/upload"
	serviceupload "github.com/Tidehyc/gorangefeed/internal/service/upload"
)

// In-memory mocks for testing Service use cases

type mockSessionRepo struct {
	mu       sync.Mutex
	sessions map[string]*domainupload.UploadSession
}

func newMockSessionRepo() *mockSessionRepo {
	return &mockSessionRepo{
		sessions: make(map[string]*domainupload.UploadSession),
	}
}

func (m *mockSessionRepo) Create(ctx context.Context, session *domainupload.UploadSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.UploadID] = session
	return nil
}

func (m *mockSessionRepo) FindByUploadID(ctx context.Context, uploadID string) (*domainupload.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[uploadID]
	if !ok {
		return nil, domainupload.ErrSessionNotFound
	}
	cp := *s
	return &cp, nil
}

func (m *mockSessionRepo) FindActiveByUserAndFileHash(ctx context.Context, userID int64, fileHash string) (*domainupload.UploadSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, s := range m.sessions {
		if s.UserID == userID && s.FileHash == fileHash && s.IsActive() {
			cp := *s
			return &cp, nil
		}
	}
	return nil, domainupload.ErrSessionNotFound
}

func (m *mockSessionRepo) TransitionStatus(ctx context.Context, uploadID string, expectedStatus, newStatus domainupload.UploadStatus, playURL string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[uploadID]
	if !ok || s.Status != expectedStatus {
		return false, nil
	}
	s.Status = newStatus
	if playURL != "" {
		s.PlayURL = playURL
	}
	return true, nil
}

type mockBitmapStore struct {
	mu     sync.Mutex
	bits   map[string]map[int]bool
}

func newMockBitmapStore() *mockBitmapStore {
	return &mockBitmapStore{
		bits: make(map[string]map[int]bool),
	}
}

func (m *mockBitmapStore) SetChunk(ctx context.Context, uploadID string, chunkIndex int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bits[uploadID] == nil {
		m.bits[uploadID] = make(map[int]bool)
	}
	m.bits[uploadID][chunkIndex] = true
	return nil
}

func (m *mockBitmapStore) IsChunkUploaded(ctx context.Context, uploadID string, chunkIndex int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.bits[uploadID] == nil {
		return false, nil
	}
	return m.bits[uploadID][chunkIndex], nil
}

func (m *mockBitmapStore) CountUploadedChunks(ctx context.Context, uploadID string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.bits[uploadID]), nil
}

func (m *mockBitmapStore) GetUploadedChunks(ctx context.Context, uploadID string, totalChunks int) ([]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []int
	for i := 0; i < totalChunks; i++ {
		if m.bits[uploadID] != nil && m.bits[uploadID][i] {
			res = append(res, i)
		}
	}
	return res, nil
}

func (m *mockBitmapStore) DeleteBitmap(ctx context.Context, uploadID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.bits, uploadID)
	return nil
}

type mockMergeLocker struct {
	mu     sync.Mutex
	locked map[string]bool
}

func newMockMergeLocker() *mockMergeLocker {
	return &mockMergeLocker{locked: make(map[string]bool)}
}

func (m *mockMergeLocker) TryLock(ctx context.Context, uploadID string, ttl time.Duration) (func() error, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.locked[uploadID] {
		return nil, domainupload.ErrMergeConflict
	}
	m.locked[uploadID] = true
	unlock := func() error {
		m.mu.Lock()
		defer m.mu.Unlock()
		delete(m.locked, uploadID)
		return nil
	}
	return unlock, nil
}

type mockStorage struct {
	savedChunks map[string]map[int][]byte
	mergeURL    string
}

func newMockStorage() *mockStorage {
	return &mockStorage{
		savedChunks: make(map[string]map[int][]byte),
		mergeURL:    "/media/videos/101/20261026/uuid.mp4",
	}
}

func (m *mockStorage) SaveChunk(uploadID string, chunkIndex int, expectedMD5 string, src io.Reader) (int64, error) {
	b, err := io.ReadAll(src)
	if err != nil {
		return 0, err
	}
	if m.savedChunks[uploadID] == nil {
		m.savedChunks[uploadID] = make(map[int][]byte)
	}
	m.savedChunks[uploadID][chunkIndex] = b
	return int64(len(b)), nil
}

func (m *mockStorage) MergeChunks(uploadID string, totalChunks int, authorID int64) (string, int64, error) {
	var total int64
	for i := 0; i < totalChunks; i++ {
		total += int64(len(m.savedChunks[uploadID][i]))
	}
	return m.mergeURL, total, nil
}

func (m *mockStorage) CleanSession(uploadID string) error {
	delete(m.savedChunks, uploadID)
	return nil
}

func TestService_InitAndResume(t *testing.T) {
	repo := newMockSessionRepo()
	bitmap := newMockBitmapStore()
	locker := newMockMergeLocker()
	storage := newMockStorage()
	svc := serviceupload.NewService(repo, bitmap, locker, storage)

	ctx := context.Background()
	userID := int64(101)
	fileHash := "hash_abc_123"

	// 1. Initial create
	initRes, err := svc.InitSession(ctx, userID, "video.mp4", 15*1024*1024, 5*1024*1024, 3, fileHash)
	if err != nil {
		t.Fatalf("failed to init session: %v", err)
	}
	if initRes.IsResumed {
		t.Fatalf("expected new session, got resumed")
	}
	if initRes.UploadID == "" || initRes.TotalChunks != 3 {
		t.Fatalf("unexpected init response: %+v", initRes)
	}

	// 2. Upload chunk 0
	err = svc.UploadChunk(ctx, userID, initRes.UploadID, 0, "hash0", bytes.NewReader([]byte("chunk0")))
	if err != nil {
		t.Fatalf("failed to upload chunk 0: %v", err)
	}

	// 3. Second Init with same fileHash -> should resume and return existing UploadID & uploaded chunks
	resumeRes, err := svc.InitSession(ctx, userID, "video.mp4", 15*1024*1024, 5*1024*1024, 3, fileHash)
	if err != nil {
		t.Fatalf("failed to resume session: %v", err)
	}
	if !resumeRes.IsResumed {
		t.Fatalf("expected resumed session")
	}
	if resumeRes.UploadID != initRes.UploadID {
		t.Fatalf("expected uploadID %s, got %s", initRes.UploadID, resumeRes.UploadID)
	}
	if len(resumeRes.UploadedChunks) != 1 || resumeRes.UploadedChunks[0] != 0 {
		t.Fatalf("expected [0], got %+v", resumeRes.UploadedChunks)
	}
}

func TestService_UploadChunk_PermissionsAndBounds(t *testing.T) {
	repo := newMockSessionRepo()
	bitmap := newMockBitmapStore()
	locker := newMockMergeLocker()
	storage := newMockStorage()
	svc := serviceupload.NewService(repo, bitmap, locker, storage)

	ctx := context.Background()
	authorID := int64(201)
	hackerID := int64(999)

	initRes, _ := svc.InitSession(ctx, authorID, "clip.mp4", 10*1024*1024, 5*1024*1024, 2, "hash_clip")

	// 1. Hacker tries to upload chunk to author's session -> 403 Forbidden
	err := svc.UploadChunk(ctx, hackerID, initRes.UploadID, 0, "hash0", bytes.NewReader([]byte("c0")))
	if err != domainupload.ErrSessionForbidden {
		t.Fatalf("expected ErrSessionForbidden, got %v", err)
	}

	// 2. Out of bounds chunk index
	err = svc.UploadChunk(ctx, authorID, initRes.UploadID, 99, "hash99", bytes.NewReader([]byte("c99")))
	if err != domainupload.ErrInvalidChunkIndex {
		t.Fatalf("expected ErrInvalidChunkIndex, got %v", err)
	}

	// 3. Negative chunk index
	err = svc.UploadChunk(ctx, authorID, initRes.UploadID, -1, "hash-1", bytes.NewReader([]byte("c-1")))
	if err != domainupload.ErrInvalidChunkIndex {
		t.Fatalf("expected ErrInvalidChunkIndex, got %v", err)
	}
}

func TestService_Complete_Flow(t *testing.T) {
	repo := newMockSessionRepo()
	bitmap := newMockBitmapStore()
	locker := newMockMergeLocker()
	storage := newMockStorage()
	svc := serviceupload.NewService(repo, bitmap, locker, storage)

	ctx := context.Background()
	userID := int64(301)

	initRes, _ := svc.InitSession(ctx, userID, "full.mp4", 10*1024*1024, 5*1024*1024, 2, "hash_full")

	// 1. Try complete before uploading all chunks -> ErrMissingChunks
	_, err := svc.Complete(ctx, userID, initRes.UploadID)
	if err != domainupload.ErrMissingChunks {
		t.Fatalf("expected ErrMissingChunks, got %v", err)
	}

	// 2. Upload chunk 0
	_ = svc.UploadChunk(ctx, userID, initRes.UploadID, 0, "h0", bytes.NewReader([]byte("c0")))

	// Still missing chunk 1
	_, err = svc.Complete(ctx, userID, initRes.UploadID)
	if err != domainupload.ErrMissingChunks {
		t.Fatalf("expected ErrMissingChunks, got %v", err)
	}

	// 3. Upload chunk 1
	_ = svc.UploadChunk(ctx, userID, initRes.UploadID, 1, "h1", bytes.NewReader([]byte("c1")))

	// 4. Complete successfully
	compRes, err := svc.Complete(ctx, userID, initRes.UploadID)
	if err != nil {
		t.Fatalf("expected complete to succeed, got %v", err)
	}
	if compRes.PlayURL != storage.mergeURL {
		t.Fatalf("expected playURL %s, got %s", storage.mergeURL, compRes.PlayURL)
	}

	// 5. Repeat Complete -> idempotent fast-path returns existing PlayURL
	compRes2, err := svc.Complete(ctx, userID, initRes.UploadID)
	if err != nil || compRes2.PlayURL != storage.mergeURL {
		t.Fatalf("expected idempotent replay to succeed, got %+v (err=%v)", compRes2, err)
	}

	// 6. GetStatus after completion
	stat, err := svc.GetStatus(ctx, userID, initRes.UploadID)
	if err != nil {
		t.Fatalf("failed to get status: %v", err)
	}
	if stat.Status != domainupload.UploadStatusCompleted || stat.PlayURL != storage.mergeURL {
		t.Fatalf("unexpected status: %+v", stat)
	}
}
