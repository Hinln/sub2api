package routes

import (
	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

// Keep theme registration separate so upstream route changes rarely overlap.
func registerThemeRoutes(v1, admin *gin.RouterGroup, h *handler.Handlers, stepUpAuth middleware.StepUpAuthMiddleware, panelRateLimiter *middleware.PanelRateLimiter) {
	v1.GET("/theme", panelRateLimiter.PublicIP(), h.Setting.GetActiveTheme)
	themes := admin.Group("/themes")
	themes.GET("", h.Admin.Setting.ListThemes)
	themes.POST("/upload", gin.HandlerFunc(stepUpAuth), h.Admin.Setting.UploadTheme)
	themes.PUT("/active", gin.HandlerFunc(stepUpAuth), h.Admin.Setting.ActivateTheme)
}
