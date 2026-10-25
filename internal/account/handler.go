package account

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/Tidehyc/gorangefeed/internal/infra/response"
	"github.com/Tidehyc/gorangefeed/internal/infra/storage"
	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for the account domain.
type Handler struct {
	svc           Service
	jwtMgr        *jwt.TokenManager
	avatarStorage *storage.AvatarStorage
}

// NewHandler creates a new account Handler.
func NewHandler(svc Service, jwtMgr *jwt.TokenManager, avatarStorage *storage.AvatarStorage) *Handler {
	return &Handler{
		svc:           svc,
		jwtMgr:        jwtMgr,
		avatarStorage: avatarStorage,
	}
}

// Register handles POST /api/v1/auth/register.
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request format"))
		return
	}

	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || password == "" {
		response.Fail(c, response.ErrInvalidParam("Username and password are required"))
		return
	}

	user, err := h.svc.Register(c.Request.Context(), username, password)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.Success(c, http.StatusCreated, ToUserProfileResponse(user))
}

// Login handles POST /api/v1/auth/login.
func (h *Handler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request format"))
		return
	}

	username := strings.TrimSpace(req.Username)
	password := req.Password
	if username == "" || password == "" {
		response.Fail(c, response.ErrInvalidParam("Username and password are required"))
		return
	}

	tokens, user, err := h.svc.Login(c.Request.Context(), username, password)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, LoginResponse{
		Tokens: *tokens,
		User:   ToUserProfileResponse(user),
	})
}

// RefreshToken handles POST /api/v1/auth/refresh.
func (h *Handler) RefreshToken(c *gin.Context) {
	var req RefreshRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request format"))
		return
	}

	refreshToken := strings.TrimSpace(req.RefreshToken)
	if refreshToken == "" {
		response.Fail(c, response.ErrInvalidParam("refresh_token is required"))
		return
	}

	tokens, err := h.svc.RefreshToken(c.Request.Context(), refreshToken)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, tokens)
}

// Logout handles POST /api/v1/auth/logout.
func (h *Handler) Logout(c *gin.Context) {
	var req LogoutRequest
	_ = c.ShouldBindJSON(&req)

	sessionID := ""
	if req.RefreshToken != "" {
		claims, err := h.jwtMgr.VerifyRefreshToken(req.RefreshToken)
		if err == nil && claims != nil {
			sessionID = claims.SessionID
		}
	}

	accessToken := middleware.ExtractToken(c)
	_ = h.svc.Logout(c.Request.Context(), sessionID, accessToken)

	response.OK(c, gin.H{"message": "Logged out successfully"})
}

// GetMe handles GET /api/v1/users/me (requires JWTAuth).
func (h *Handler) GetMe(c *gin.Context) {
	userID := middleware.MustGetUserID(c)

	user, err := h.svc.GetProfile(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, ToUserProfileResponse(user))
}

// UpdateMe handles PUT /api/v1/users/me (requires JWTAuth).
func (h *Handler) UpdateMe(c *gin.Context) {
	userID := middleware.MustGetUserID(c)

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParam("Invalid request format"))
		return
	}

	nickname := strings.TrimSpace(req.Nickname)
	bio := strings.TrimSpace(req.Bio)
	if nickname == "" && bio == "" {
		response.Fail(c, response.ErrInvalidParam("At least nickname or bio must be provided"))
		return
	}

	user, err := h.svc.UpdateProfile(c.Request.Context(), userID, nickname, bio)
	if err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, ToUserProfileResponse(user))
}

// UploadAvatar handles POST /api/v1/users/avatar (requires JWTAuth).
func (h *Handler) UploadAvatar(c *gin.Context) {
	userID := middleware.MustGetUserID(c)

	if h.avatarStorage == nil {
		response.Fail(c, response.ErrInternal("Avatar storage not configured"))
		return
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Fail(c, response.ErrInvalidParam("Missing avatar file in 'file' form-data field"))
		return
	}

	avatarURL, err := h.avatarStorage.SaveAvatar(userID, fileHeader)
	if err != nil {
		response.Fail(c, err)
		return
	}

	if err := h.svc.UpdateAvatar(c.Request.Context(), userID, avatarURL); err != nil {
		response.Fail(c, err)
		return
	}

	response.OK(c, gin.H{"avatar_url": avatarURL})
}

// GetPublicProfile handles GET /api/v1/users/:userId (soft auth).
func (h *Handler) GetPublicProfile(c *gin.Context) {
	rawID := c.Param("userId")
	targetID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || targetID <= 0 {
		response.Fail(c, response.ErrInvalidParam("Invalid userId"))
		return
	}

	user, err := h.svc.GetProfile(c.Request.Context(), targetID)
	if err != nil {
		response.Fail(c, err)
		return
	}

	if user.Status != UserStatusActive {
		response.Fail(c, response.ErrNotFound("User not found"))
		return
	}

	response.OK(c, ToPublicUserProfileResponse(user))
}
