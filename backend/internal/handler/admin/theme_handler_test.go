//go:build unit

package admin

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/Wei-Shaw/sub2api/internal/theme"
	"github.com/gin-gonic/gin"
)

type adminThemeRepo struct {
	values map[string]string
	fail   error
}

func (r *adminThemeRepo) Get(context.Context, string) (*service.Setting, error) {
	panic("unexpected Get")
}
func (r *adminThemeRepo) GetValue(_ context.Context, key string) (string, error) {
	if r.fail != nil {
		return "", r.fail
	}
	value, ok := r.values[key]
	if !ok {
		return "", service.ErrSettingNotFound
	}
	return value, nil
}
func (r *adminThemeRepo) Set(_ context.Context, key, value string) error {
	if r.fail != nil {
		return r.fail
	}
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	return nil
}
func (r *adminThemeRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple")
}
func (r *adminThemeRepo) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple")
}
func (r *adminThemeRepo) Delete(context.Context, string) error { panic("unexpected Delete") }
func (r *adminThemeRepo) GetAll(context.Context) (map[string]string, error) {
	if r.fail != nil {
		return nil, r.fail
	}
	return r.values, nil
}

func adminThemeHandler(repo *adminThemeRepo) *SettingHandler {
	return NewSettingHandler(service.NewSettingService(repo, &config.Config{}), nil, nil, nil, nil, nil, nil)
}
func adminThemeRequest(t *testing.T, h *SettingHandler, method, path, body, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	if contentType != "" {
		c.Request.Header.Set("Content-Type", contentType)
	}
	switch path {
	case "/themes":
		h.ListThemes(c)
	case "/themes/upload":
		h.UploadTheme(c)
	case "/themes/active":
		h.ActivateTheme(c)
	default:
		t.Fatal("unexpected test path")
	}
	return w
}
func adminThemeMultipart(t *testing.T, filename string, archive []byte, extra bool) (string, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	f, err := w.CreateFormFile("theme", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.Write(archive); err != nil {
		t.Fatal(err)
	}
	if extra {
		if err := w.WriteField("extra", "no"); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.String(), w.FormDataContentType()
}
func adminThemeArchive(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("theme.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.WriteString(f, `{"schema_version":1,"id":"ocean","name":"Ocean","version":"1.0.0","tokens":{"font":"serif"}}`); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestAdminThemeUploadListActivateAndReset(t *testing.T) {
	repo := &adminThemeRepo{values: map[string]string{"smtp_password": "private-secret"}}
	h := adminThemeHandler(repo)
	body, contentType := adminThemeMultipart(t, "ocean.s2theme", adminThemeArchive(t), false)
	w := adminThemeRequest(t, h, http.MethodPost, "/themes/upload", body, contentType)
	if w.Code != http.StatusOK {
		t.Fatalf("upload: %d %s", w.Code, w.Body.String())
	}
	var upload struct {
		Data theme.Package `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &upload); err != nil {
		t.Fatal(err)
	}
	if !theme.ValidPackageID(upload.Data.ID) {
		t.Fatal("missing package id")
	}
	w = adminThemeRequest(t, h, http.MethodGet, "/themes", "", "")
	if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "private-secret") || !strings.Contains(w.Body.String(), `"active_theme":null`) {
		t.Fatalf("list: %d %s", w.Code, w.Body.String())
	}
	w = adminThemeRequest(t, h, http.MethodPut, "/themes/active", `{"theme_id":"`+upload.Data.ID+`"}`, "application/json")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), upload.Data.ID) {
		t.Fatalf("activate: %d %s", w.Code, w.Body.String())
	}
	w = adminThemeRequest(t, h, http.MethodPut, "/themes/active", `{"theme_id":null}`, "application/json")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"active_theme":null`) {
		t.Fatalf("reset: %d %s", w.Code, w.Body.String())
	}
}

func TestAdminThemeUploadRejectsInvalidMultipartAndArchives(t *testing.T) {
	for _, test := range []struct {
		name, filename string
		data           []byte
		extra          bool
		want           int
	}{
		{"extension", "ocean.exe", adminThemeArchive(t), false, http.StatusBadRequest},
		{"bad zip", "ocean.zip", []byte("bad zip"), false, http.StatusBadRequest},
		{"extra part", "ocean.zip", adminThemeArchive(t), true, http.StatusBadRequest},
		{"large package", "ocean.zip", make([]byte, theme.MaxPackageBytes+1), false, http.StatusRequestEntityTooLarge},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := &adminThemeRepo{}
			body, contentType := adminThemeMultipart(t, test.filename, test.data, test.extra)
			w := adminThemeRequest(t, adminThemeHandler(repo), http.MethodPost, "/themes/upload", body, contentType)
			if w.Code != test.want {
				t.Fatalf("got %d want %d: %s", w.Code, test.want, w.Body.String())
			}
			if len(repo.values) != 0 {
				t.Fatal("invalid upload persisted state")
			}
		})
	}
	w := adminThemeRequest(t, adminThemeHandler(&adminThemeRepo{}), http.MethodPost, "/themes/upload", `{}`, "application/json")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid content type: %d", w.Code)
	}
}

func TestAdminThemeActivationRejectsAmbiguousRequests(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `[]`, `{"other":null}`, `{"theme_id":1}`, `{"theme_id":false}`, `{"theme_id":{}}`, `{"theme_id":null,"other":1}`, `{"theme_id":null,"theme_id":null}`, `{"theme_id":null}{}`, `{"theme_id":"../escape"}`, `{"theme_id":"` + strings.Repeat("x", 2048) + `"}`} {
		repo := &adminThemeRepo{}
		w := adminThemeRequest(t, adminThemeHandler(repo), http.MethodPut, "/themes/active", body, "application/json")
		if w.Code != http.StatusBadRequest {
			t.Fatalf("accepted body %s: %d %s", body, w.Code, w.Body.String())
		}
		if len(repo.values) != 0 {
			t.Fatal("invalid request changed active theme")
		}
	}
	w := adminThemeRequest(t, adminThemeHandler(&adminThemeRepo{}), http.MethodPut, "/themes/active", `{"theme_id":"`+strings.Repeat("a", 64)+`"}`, "application/json")
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown package: %d", w.Code)
	}
}

func TestAdminThemeDatabaseFailuresUseErrorEnvelope(t *testing.T) {
	h := adminThemeHandler(&adminThemeRepo{fail: errors.New("private database diagnostic")})
	for _, path := range []string{"/themes", "/themes/active"} {
		body := ""
		method := http.MethodGet
		if path == "/themes/active" {
			body = `{"theme_id":null}`
			method = http.MethodPut
		}
		w := adminThemeRequest(t, h, method, path, body, "application/json")
		if w.Code != http.StatusInternalServerError || strings.Contains(w.Body.String(), "private database diagnostic") {
			t.Fatalf("bad error response: %d %s", w.Code, w.Body.String())
		}
	}
}
