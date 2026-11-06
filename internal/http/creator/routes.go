package creator

import (
	domainaccount "github.com/Tidehyc/gorangefeed/internal/domain/account"
	"github.com/Tidehyc/gorangefeed/internal/infra/jwt"
	"github.com/Tidehyc/gorangefeed/internal/infra/middleware"
	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RegisterRoutes registers all Creator Center routes under /api/v1/creator.
func RegisterRoutes(r *gin.Engine, h *Handler, jwtMgr *jwt.TokenManager, tokenStore domainaccount.TokenStore, rdb *redis.Client) {
	v1 := r.Group("/api/v1/creator")

	jwtAuth := middleware.JWTAuth(jwtMgr, tokenStore)

	// Draft management
	v1.POST("/drafts", jwtAuth, h.SaveDraft)
	v1.GET("/drafts", jwtAuth, h.ListDrafts)
	v1.GET("/drafts/:id", jwtAuth, h.GetDraft)
	v1.PUT("/drafts/:id", jwtAuth, h.UpdateDraft)
	v1.DELETE("/drafts/:id", jwtAuth, h.DeleteDraft)
	v1.POST("/drafts/:id/publish", jwtAuth, h.PublishDraft)

	// Scheduled publishing
	v1.POST("/publish-scheduled", jwtAuth, h.SchedulePublish)
	v1.GET("/scheduled-tasks", jwtAuth, h.ListScheduledTasks)
	v1.DELETE("/scheduled-tasks/:id", jwtAuth, h.CancelScheduledTask)
}
