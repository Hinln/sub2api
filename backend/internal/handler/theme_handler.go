package handler

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/gin-gonic/gin"
)

// GetActiveTheme exposes only the selected, validated public manifest. Other
// settings and installed theme packages never enter the public response.
func (h *SettingHandler) GetActiveTheme(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	pkg, err := h.settingService.GetActiveTheme(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"active_theme": pkg})
}
