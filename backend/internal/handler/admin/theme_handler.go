package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/theme"
	"github.com/gin-gonic/gin"
)

func (h *SettingHandler) ListThemes(c *gin.Context) {
	result, err := h.settingService.ListThemes(c.Request.Context())
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, result)
}

func (h *SettingHandler) UploadTheme(c *gin.Context) {
	// Stream multipart data without ParseMultipartForm's temporary disk files.
	const maxUploadBytes = theme.MaxPackageBytes + (16 << 10)
	if c.Request.ContentLength > maxUploadBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "theme upload exceeds the request size limit")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxUploadBytes)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		response.BadRequest(c, "upload must be multipart/form-data with one theme file")
		return
	}
	part, err := reader.NextPart()
	if err != nil {
		themeUploadError(c, err)
		return
	}
	defer func() { _ = part.Close() }()
	if part.FormName() != "theme" || part.FileName() == "" {
		response.BadRequest(c, "a theme file is required in the theme field")
		return
	}
	ext := strings.ToLower(filepath.Ext(part.FileName()))
	if ext != ".s2theme" && ext != ".zip" {
		response.BadRequest(c, "theme file extension must be .s2theme or .zip")
		return
	}
	archive, err := io.ReadAll(io.LimitReader(part, theme.MaxPackageBytes+1))
	if err != nil {
		themeUploadError(c, err)
		return
	}
	if len(archive) > theme.MaxPackageBytes {
		response.Error(c, http.StatusRequestEntityTooLarge, "theme package exceeds the 256 KiB limit")
		return
	}
	if _, err = reader.NextPart(); err != io.EOF {
		if err != nil {
			themeUploadError(c, err)
		} else {
			response.BadRequest(c, "upload must contain only one theme file")
		}
		return
	}
	pkg, err := h.settingService.InstallTheme(c.Request.Context(), archive)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, pkg)
}

func themeUploadError(c *gin.Context, err error) {
	var limitErr *http.MaxBytesError
	if errors.As(err, &limitErr) {
		response.Error(c, http.StatusRequestEntityTooLarge, "theme upload exceeds the request size limit")
		return
	}
	response.BadRequest(c, "invalid multipart theme upload")
}

func (h *SettingHandler) ActivateTheme(c *gin.Context) {
	// Accept exactly {"theme_id": string|null}; explicit null restores defaults.
	d := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1024))
	first, err := d.Token()
	if err != nil || first != json.Delim('{') {
		response.BadRequest(c, "request must contain theme_id as a string or null")
		return
	}
	key, err := d.Token()
	if err != nil || key != "theme_id" {
		response.BadRequest(c, "theme_id is required")
		return
	}
	var id *string
	if err := d.Decode(&id); err != nil {
		response.BadRequest(c, "theme_id must be a string or null")
		return
	}
	end, err := d.Token()
	if err != nil || end != json.Delim('}') {
		response.BadRequest(c, "request must contain only one theme_id field")
		return
	}
	if _, err := d.Token(); err != io.EOF {
		response.BadRequest(c, "request must contain one JSON object")
		return
	}
	selected := ""
	if id != nil {
		selected = *id
	}
	pkg, err := h.settingService.ActivateTheme(c.Request.Context(), selected)
	if response.ErrorFrom(c, err) {
		return
	}
	response.Success(c, gin.H{"active_theme": pkg})
}
