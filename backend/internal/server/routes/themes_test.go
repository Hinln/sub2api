package routes

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/theme"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// These route tests deliberately use only theme routes, real settings handlers,
// and an in-memory repository. No application wiring or external stores run.
type themeRouteSettingsRepo struct {
	values        map[string]string
	reads, writes int
}

func (r *themeRouteSettingsRepo) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get")
}

func (r *themeRouteSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	r.reads++
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}

func (r *themeRouteSettingsRepo) Set(_ context.Context, key, value string) error {
	r.writes++
	r.values[key] = value
	return nil
}

func (r *themeRouteSettingsRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple")
}

func (r *themeRouteSettingsRepo) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple")
}

func (r *themeRouteSettingsRepo) GetAll(context.Context) (map[string]string, error) {
	r.reads++
	values := make(map[string]string, len(r.values))
	for key, value := range r.values {
		values[key] = value
	}
	return values, nil
}

func (r *themeRouteSettingsRepo) Delete(context.Context, string) error {
	panic("theme routes must not delete settings")
}

type themeRouteAudit struct {
	method, path string
	status       int
}

type themeRouteFixture struct {
	router                 *gin.Engine
	repo                   *themeRouteSettingsRepo
	authCalls, stepUpCalls int
	audits                 []themeRouteAudit
}

func newThemeRouteFixture(t *testing.T) *themeRouteFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	f := &themeRouteFixture{router: gin.New(), repo: &themeRouteSettingsRepo{values: map[string]string{"smtp_password": "private-setting-must-not-leak"}}}
	settings := service.NewSettingService(f.repo, &config.Config{})
	handlers := &handler.Handlers{
		Setting: handler.NewSettingHandler(settings, "test"),
		Admin:   &handler.AdminHandlers{Setting: adminhandler.NewSettingHandler(settings, nil, nil, nil, nil, nil, nil)},
	}
	auth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		f.authCalls++
		switch c.GetHeader("Authorization") {
		case "Bearer admin":
			c.Next()
		case "":
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "authentication required"})
		default:
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "administrator required"})
		}
	})
	audit := servermiddleware.AuditLogMiddleware(func(c *gin.Context) {
		c.Next()
		f.audits = append(f.audits, themeRouteAudit{c.Request.Method, c.FullPath(), c.Writer.Status()})
	})
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) {
		f.stepUpCalls++
		if c.GetHeader("X-Test-Step-Up") != "verified" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "step-up required"})
			return
		}
		c.Next()
	})
	v1 := f.router.Group("/api/v1")
	admin := v1.Group("/admin", gin.HandlerFunc(auth), gin.HandlerFunc(audit))
	registerThemeRoutes(v1, admin, handlers, stepUp, nil)
	return f
}

func (f *themeRouteFixture) request(method, path, body, contentType, authorization string, stepUp bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	if stepUp {
		request.Header.Set("X-Test-Step-Up", "verified")
	}
	recorder := httptest.NewRecorder()
	f.router.ServeHTTP(recorder, request)
	return recorder
}

func themeRouteUpload(t *testing.T) (string, string) {
	t.Helper()
	var archive bytes.Buffer
	zw := zip.NewWriter(&archive)
	manifest, err := zw.Create("theme.json")
	require.NoError(t, err)
	_, err = io.WriteString(manifest, `{"schema_version":1,"id":"route-theme","name":"Route Theme","version":"1.0.0","tokens":{"font":"serif","radius_scale":1.25}}`)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	file, err := mw.CreateFormFile("theme", "route-theme.s2theme")
	require.NoError(t, err)
	_, err = file.Write(archive.Bytes())
	require.NoError(t, err)
	require.NoError(t, mw.Close())
	return body.String(), mw.FormDataContentType()
}

func TestThemeRoutesRequireAdminBeforeAuditStepUpAndHandlers(t *testing.T) {
	archive, contentType := themeRouteUpload(t)
	for _, endpoint := range []struct{ method, path, body, contentType string }{
		{http.MethodGet, "/api/v1/admin/themes", "", ""},
		{http.MethodPost, "/api/v1/admin/themes/upload", archive, contentType},
		{http.MethodPut, "/api/v1/admin/themes/active", `{"theme_id":null}`, "application/json"},
	} {
		for _, authorization := range []string{"", "Bearer ordinary-user"} {
			t.Run(endpoint.method+endpoint.path+authorization, func(t *testing.T) {
				f := newThemeRouteFixture(t)
				response := f.request(endpoint.method, endpoint.path, endpoint.body, endpoint.contentType, authorization, true)
				want := http.StatusUnauthorized
				if authorization != "" {
					want = http.StatusForbidden
				}
				require.Equal(t, want, response.Code)
				require.Equal(t, 1, f.authCalls)
				require.Zero(t, f.stepUpCalls)
				require.Empty(t, f.audits)
				require.Zero(t, f.repo.reads, "unauthorized request reached a real settings handler")
				require.Zero(t, f.repo.writes)
			})
		}
	}
}

func TestThemeRoutesRequireStepUpForUploadAndActivation(t *testing.T) {
	archive, contentType := themeRouteUpload(t)
	for _, endpoint := range []struct{ method, path, body, contentType string }{
		{http.MethodPost, "/api/v1/admin/themes/upload", archive, contentType},
		{http.MethodPut, "/api/v1/admin/themes/active", `{"theme_id":null}`, "application/json"},
	} {
		t.Run(endpoint.path, func(t *testing.T) {
			f := newThemeRouteFixture(t)
			response := f.request(endpoint.method, endpoint.path, endpoint.body, endpoint.contentType, "Bearer admin", false)
			require.Equal(t, http.StatusForbidden, response.Code)
			require.Equal(t, 1, f.authCalls)
			require.Equal(t, 1, f.stepUpCalls)
			require.Equal(t, []themeRouteAudit{{endpoint.method, endpoint.path, http.StatusForbidden}}, f.audits)
			require.Zero(t, f.repo.reads, "request without step-up reached a settings handler")
			require.Zero(t, f.repo.writes)
		})
	}
	// Listing is read-only: administrator authentication suffices.
	f := newThemeRouteFixture(t)
	response := f.request(http.MethodGet, "/api/v1/admin/themes", "", "", "Bearer admin", false)
	require.Equal(t, http.StatusOK, response.Code)
	require.Zero(t, f.stepUpCalls)
	require.Equal(t, []themeRouteAudit{{http.MethodGet, "/api/v1/admin/themes", http.StatusOK}}, f.audits)
	require.NotContains(t, response.Body.String(), "private-setting-must-not-leak")
}

func TestThemeRoutesPublicReadDoesNotRequireAdmin(t *testing.T) {
	f := newThemeRouteFixture(t)
	response := f.request(http.MethodGet, "/api/v1/theme", "", "", "", false)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"active_theme":null}}`, response.Body.String())
	require.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	require.Zero(t, f.authCalls)
	require.Zero(t, f.stepUpCalls)
	require.Empty(t, f.audits)
	require.Zero(t, f.repo.writes)
}

func TestThemeRoutesUploadActivatePublishAndRestoreDefault(t *testing.T) {
	f := newThemeRouteFixture(t)
	archive, contentType := themeRouteUpload(t)
	response := f.request(http.MethodPost, "/api/v1/admin/themes/upload", archive, contentType, "Bearer admin", true)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	var uploaded struct {
		Data theme.Package `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &uploaded))
	require.True(t, theme.ValidPackageID(uploaded.Data.ID))
	require.Equal(t, "route-theme", uploaded.Data.Manifest.ID)
	_, activeSettingExists := f.repo.values[service.SettingKeyActiveTheme]
	require.False(t, activeSettingExists, "upload must not implicitly activate a theme")

	response = f.request(http.MethodGet, "/api/v1/admin/themes", "", "", "Bearer admin", false)
	require.Equal(t, http.StatusOK, response.Code)
	var listed struct {
		Data service.ThemeList `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &listed))
	require.Nil(t, listed.Data.ActiveTheme)
	require.Len(t, listed.Data.Themes, 1)
	require.Equal(t, uploaded.Data.ID, listed.Data.Themes[0].ID)

	response = f.request(http.MethodGet, "/api/v1/theme", "", "", "", false)
	require.Contains(t, response.Body.String(), `"active_theme":null`)
	response = f.request(http.MethodPut, "/api/v1/admin/themes/active", `{"theme_id":"`+uploaded.Data.ID+`"}`, "application/json", "Bearer admin", true)
	require.Equal(t, http.StatusOK, response.Code, response.Body.String())
	require.Equal(t, uploaded.Data.ID, f.repo.values[service.SettingKeyActiveTheme])

	response = f.request(http.MethodGet, "/api/v1/theme", "", "", "", false)
	require.Equal(t, http.StatusOK, response.Code)
	var public struct {
		Data struct {
			ActiveTheme *theme.Package `json:"active_theme"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &public))
	require.NotNil(t, public.Data.ActiveTheme)
	require.Equal(t, uploaded.Data.ID, public.Data.ActiveTheme.ID)
	require.NotContains(t, response.Body.String(), "private-setting-must-not-leak")
	require.NotContains(t, response.Body.String(), `"themes"`)

	response = f.request(http.MethodPut, "/api/v1/admin/themes/active", `{"theme_id":null}`, "application/json", "Bearer admin", true)
	require.Equal(t, http.StatusOK, response.Code)
	response = f.request(http.MethodGet, "/api/v1/theme", "", "", "", false)
	require.Equal(t, http.StatusOK, response.Code)
	require.JSONEq(t, `{"code":0,"message":"success","data":{"active_theme":null}}`, response.Body.String())
	require.Equal(t, 4, f.authCalls, "only the four administrator requests enter admin authentication")
	require.Equal(t, 3, f.stepUpCalls, "only upload and the two activation changes require step-up")
	require.Equal(t, []themeRouteAudit{
		{http.MethodPost, "/api/v1/admin/themes/upload", http.StatusOK},
		{http.MethodGet, "/api/v1/admin/themes", http.StatusOK},
		{http.MethodPut, "/api/v1/admin/themes/active", http.StatusOK},
		{http.MethodPut, "/api/v1/admin/themes/active", http.StatusOK},
	}, f.audits)
}
