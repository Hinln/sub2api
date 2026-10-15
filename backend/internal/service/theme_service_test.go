//go:build unit

package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/theme"
)

type themeSettingsRepo struct {
	mu                         sync.Mutex
	values                     map[string]string
	readErr, writeErr, listErr error
	readErrors                 map[string]error
	sets                       int
}

func (r *themeSettingsRepo) Get(context.Context, string) (*Setting, error) { panic("unexpected Get") }
func (r *themeSettingsRepo) GetValue(_ context.Context, key string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.readErr != nil {
		return "", r.readErr
	}
	if err := r.readErrors[key]; err != nil {
		return "", err
	}
	v, ok := r.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return v, nil
}
func (r *themeSettingsRepo) Set(_ context.Context, key, value string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.writeErr != nil {
		return r.writeErr
	}
	if r.values == nil {
		r.values = map[string]string{}
	}
	r.values[key] = value
	r.sets++
	return nil
}
func (r *themeSettingsRepo) GetMultiple(context.Context, []string) (map[string]string, error) {
	panic("unexpected GetMultiple")
}
func (r *themeSettingsRepo) SetMultiple(context.Context, map[string]string) error {
	panic("unexpected SetMultiple")
}
func (r *themeSettingsRepo) Delete(context.Context, string) error {
	panic("themes must never delete settings")
}
func (r *themeSettingsRepo) GetAll(context.Context) (map[string]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.listErr != nil {
		return nil, r.listErr
	}
	out := map[string]string{}
	for k, v := range r.values {
		out[k] = v
	}
	return out, nil
}

func themeServiceArchive(t *testing.T, id, version string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	f, err := w.Create("theme.json")
	if err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(f, `{"schema_version":1,"id":%q,"name":"Ocean","version":%q,"tokens":{"font":"system"}}`, id, version)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestThemesInstallIsIdempotentAndDoesNotActivate(t *testing.T) {
	ctx := context.Background()
	repo := &themeSettingsRepo{values: map[string]string{"smtp_password": "never-return-this"}}
	svc := &SettingService{settingRepo: repo}
	archive := themeServiceArchive(t, "ocean", "1.0.0")
	first, err := svc.InstallTheme(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.InstallTheme(ctx, archive)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || first.InstalledAt != second.InstalledAt || repo.sets != 1 {
		t.Fatal("upload must preserve an existing package")
	}
	if active, err := svc.GetActiveTheme(ctx); err != nil || active != nil {
		t.Fatalf("upload activated a theme: %#v %v", active, err)
	}
	if _, err := svc.InstallTheme(ctx, themeServiceArchive(t, "ocean", "1.0.1")); err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListThemes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Themes) != 2 || list.ActiveTheme != nil {
		t.Fatalf("unexpected list: %#v", list)
	}
	raw, _ := json.Marshal(list)
	if strings.Contains(string(raw), "never-return-this") || strings.Contains(string(raw), "smtp_password") {
		t.Fatal("list leaked unrelated settings")
	}
}

func TestThemesActivateAndRestoreDefault(t *testing.T) {
	ctx := context.Background()
	repo := &themeSettingsRepo{}
	svc := &SettingService{settingRepo: repo}
	pkg, err := svc.InstallTheme(ctx, themeServiceArchive(t, "ocean", "1.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	if selected, err := svc.ActivateTheme(ctx, pkg.ID); err != nil || selected == nil || selected.ID != pkg.ID {
		t.Fatalf("activate: %#v %v", selected, err)
	}
	if active, err := svc.GetActiveTheme(ctx); err != nil || active == nil || active.ID != pkg.ID {
		t.Fatalf("get active: %#v %v", active, err)
	}
	list, err := svc.ListThemes(ctx)
	if err != nil || list.ActiveTheme == nil || list.ActiveTheme.ID != pkg.ID {
		t.Fatalf("list active: %#v %v", list, err)
	}
	if active, err := svc.ActivateTheme(ctx, ""); err != nil || active != nil {
		t.Fatalf("restore: %#v %v", active, err)
	}
	if active, err := svc.GetActiveTheme(ctx); err != nil || active != nil {
		t.Fatalf("restore persisted: %#v %v", active, err)
	}
	if _, err := svc.ActivateTheme(ctx, "../oops"); err == nil {
		t.Fatal("accepted malformed id")
	}
	if _, err := svc.ActivateTheme(ctx, strings.Repeat("a", 64)); err == nil {
		t.Fatal("accepted unknown id")
	}
}

func TestThemesCorruptActiveSafelyFallsBack(t *testing.T) {
	id := strings.Repeat("a", 64)
	for name, values := range map[string]map[string]string{
		"absent":              {},
		"default":             {SettingKeyActiveTheme: ""},
		"malformed reference": {SettingKeyActiveTheme: "../secret"},
		"unknown package":     {SettingKeyActiveTheme: id},
		"invalid JSON":        {SettingKeyActiveTheme: id, SettingKeyThemePackagePrefix + id: `{"manifest":`},
		"null package":        {SettingKeyActiveTheme: id, SettingKeyThemePackagePrefix + id: `null`},
		"unsupported schema":  {SettingKeyActiveTheme: id, SettingKeyThemePackagePrefix + id: `{"id":"` + id + `","installed_at":"2026-10-15T00:00:00Z","manifest":{"schema_version":2,"id":"ocean","name":"Ocean","version":"1.0.0","tokens":{"font":"system"}}}`},
		"null token":          {SettingKeyActiveTheme: id, SettingKeyThemePackagePrefix + id: `{"id":"` + id + `","installed_at":"2026-10-15T00:00:00Z","manifest":{"schema_version":1,"id":"ocean","name":"Ocean","version":"1.0.0","tokens":{"font":null,"radius_scale":1}}}`},
	} {
		t.Run(name, func(t *testing.T) {
			repo := &themeSettingsRepo{values: values}
			svc := &SettingService{settingRepo: repo}
			if active, err := svc.GetActiveTheme(context.Background()); err != nil || active != nil {
				t.Fatalf("expected safe default: %#v %v", active, err)
			}
			list, err := svc.ListThemes(context.Background())
			if err != nil || list.ActiveTheme != nil {
				t.Fatalf("list default: %#v %v", list, err)
			}
			if repo.sets != 0 {
				t.Fatal("read must not mutate corrupt state")
			}
		})
	}
}

func TestThemesDatabaseErrorsAreNeverSuccessfulFallbacks(t *testing.T) {
	ctx := context.Background()
	failure := errors.New("database offline")
	archive := themeServiceArchive(t, "ocean", "1.0.0")
	t.Run("active reference read", func(t *testing.T) {
		svc := &SettingService{settingRepo: &themeSettingsRepo{readErr: failure}}
		if _, err := svc.GetActiveTheme(ctx); !errors.Is(err, failure) {
			t.Fatalf("lost DB error: %v", err)
		}
	})
	t.Run("active package read", func(t *testing.T) {
		repo := &themeSettingsRepo{values: map[string]string{SettingKeyActiveTheme: theme.PackageID(archive)}}
		// A missing package is safe, whereas repository read failures must surface.
		svc := &SettingService{settingRepo: repo}
		repo.readErrors = map[string]error{SettingKeyThemePackagePrefix + theme.PackageID(archive): failure}
		if _, err := svc.GetActiveTheme(ctx); !errors.Is(err, failure) {
			t.Fatalf("lost DB error: %v", err)
		}
	})
	t.Run("list", func(t *testing.T) {
		svc := &SettingService{settingRepo: &themeSettingsRepo{listErr: failure}}
		if _, err := svc.ListThemes(ctx); !errors.Is(err, failure) {
			t.Fatalf("lost DB error: %v", err)
		}
	})
	t.Run("install read", func(t *testing.T) {
		svc := &SettingService{settingRepo: &themeSettingsRepo{readErr: failure}}
		if _, err := svc.InstallTheme(ctx, archive); !errors.Is(err, failure) {
			t.Fatalf("lost DB error: %v", err)
		}
	})
	t.Run("install write", func(t *testing.T) {
		svc := &SettingService{settingRepo: &themeSettingsRepo{writeErr: failure}}
		if _, err := svc.InstallTheme(ctx, archive); !errors.Is(err, failure) {
			t.Fatalf("lost DB error: %v", err)
		}
	})
	t.Run("activate write", func(t *testing.T) {
		svc := &SettingService{settingRepo: &themeSettingsRepo{writeErr: failure}}
		if _, err := svc.ActivateTheme(ctx, ""); !errors.Is(err, failure) {
			t.Fatalf("lost DB error: %v", err)
		}
	})
}

func TestThemesConcurrentUploadsKeepIndependentPackages(t *testing.T) {
	const count = 32
	archives := make([][]byte, count)
	for i := range archives {
		archives[i] = themeServiceArchive(t, fmt.Sprintf("theme-%d", i), "1.0.0")
	}
	repo := &themeSettingsRepo{}
	svc := &SettingService{settingRepo: repo}
	var wg sync.WaitGroup
	errCh := make(chan error, count)
	for _, archive := range archives {
		wg.Add(1)
		go func(data []byte) {
			defer wg.Done()
			_, err := svc.InstallTheme(context.Background(), data)
			errCh <- err
		}(archive)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	list, err := svc.ListThemes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Themes) != count {
		t.Fatalf("lost packages during upload: got %d want %d", len(list.Themes), count)
	}
}

func TestThemesRejectInvalidUploadsBeforeWriting(t *testing.T) {
	repo := &themeSettingsRepo{}
	svc := &SettingService{settingRepo: repo}
	if _, err := svc.InstallTheme(context.Background(), []byte("not a zip")); err == nil {
		t.Fatal("accepted invalid theme")
	}
	if repo.sets != 0 {
		t.Fatal("invalid upload wrote settings")
	}
	archive := themeServiceArchive(t, "ocean", "1.0.0")
	repo.values = map[string]string{SettingKeyThemePackagePrefix + theme.PackageID(archive): "corrupt existing immutable record"}
	if _, err := svc.InstallTheme(context.Background(), archive); err == nil {
		t.Fatal("overwrote corrupt existing record")
	}
	if repo.sets != 0 {
		t.Fatal("existing content-addressed package was overwritten")
	}
}
