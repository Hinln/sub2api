//go:build unit

package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

type publicThemeRepo struct {
	values map[string]string
	err    error
}

func (r *publicThemeRepo) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get")
}
func (r *publicThemeRepo) GetValue(_ context.Context, key string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	v, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return v, nil
}
func (r *publicThemeRepo) Set(context.Context, string, string) error {
	panic("public read must not write")
}
func (r *publicThemeRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple")
}
func (r *publicThemeRepo) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple")
}
func (r *publicThemeRepo) GetAll(context.Context) (map[string]string, error) {
	panic("public route must not read all settings")
}
func (r *publicThemeRepo) Delete(context.Context, string) error { panic("public read must not delete") }

func TestGetActiveThemePublicProjectionAndSafeFallback(t *testing.T) {
	id := strings.Repeat("a", 64)
	valid := `{"id":"` + id + `","installed_at":"2026-10-15T00:00:00Z","manifest":{"schema_version":1,"id":"ocean","name":"Ocean","version":"1.0.0","tokens":{"font":"serif"}}}`
	for _, test := range []struct {
		name   string
		values map[string]string
		want   string
	}{
		{"default", nil, `"active_theme":null`},
		{"unknown", map[string]string{service.SettingKeyActiveTheme: id}, `"active_theme":null`},
		{"corrupt", map[string]string{service.SettingKeyActiveTheme: id, service.SettingKeyThemePackagePrefix + id: "bad JSON"}, `"active_theme":null`},
		{"active", map[string]string{service.SettingKeyActiveTheme: id, service.SettingKeyThemePackagePrefix + id: valid, "smtp_password": "secret"}, `"font":"serif"`},
	} {
		t.Run(test.name, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			h := NewSettingHandler(service.NewSettingService(&publicThemeRepo{values: test.values}, &config.Config{}), "test")
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/theme", nil)
			h.GetActiveTheme(c)
			if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), test.want) || strings.Contains(w.Body.String(), "secret") {
				t.Fatalf("unexpected projection: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestGetActiveThemePublicDatabaseFailureIsNotDefault(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewSettingHandler(service.NewSettingService(&publicThemeRepo{err: errors.New("db password secret")}, &config.Config{}), "test")
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/v1/theme", nil)
	h.GetActiveTheme(c)
	if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "secret") || strings.Contains(w.Body.String(), `"active_theme":null`) {
		t.Fatalf("unexpected DB fallback: %d %s", w.Code, w.Body.String())
	}
}
