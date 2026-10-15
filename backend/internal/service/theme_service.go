package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/theme"
)

const (
	SettingKeyThemePackagePrefix = "appearance.theme.package.v1."
	SettingKeyActiveTheme        = "appearance.theme.active.v1"
)

var errInvalidStoredTheme = errors.New("invalid stored theme")

type ThemeList struct {
	ActiveTheme *theme.Package   `json:"active_theme"`
	Themes      []*theme.Package `json:"themes"`
}

// InstallTheme stores each content-addressed package separately. Independent
// uploads therefore never overwrite a shared catalog, including across replicas.
// Uploading does not activate a theme or write any files to the server.
func (s *SettingService) InstallTheme(ctx context.Context, archive []byte) (*theme.Package, error) {
	manifest, err := theme.Parse(archive)
	if err != nil {
		return nil, infraerrors.BadRequest("INVALID_THEME_PACKAGE", err.Error())
	}
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("theme settings repository is unavailable")
	}
	id := theme.PackageID(archive)
	existing, err := s.loadTheme(ctx, id)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, ErrSettingNotFound) {
		return nil, err
	}
	pkg := &theme.Package{ID: id, Manifest: *manifest, InstalledAt: time.Now().UTC().Format(time.RFC3339Nano)}
	encoded, err := json.Marshal(pkg)
	if err != nil {
		return nil, fmt.Errorf("encode installed theme: %w", err)
	}
	if err := s.settingRepo.Set(ctx, SettingKeyThemePackagePrefix+id, string(encoded)); err != nil {
		return nil, fmt.Errorf("store installed theme: %w", err)
	}
	return pkg, nil
}

func (s *SettingService) ListThemes(ctx context.Context) (*ThemeList, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("theme settings repository is unavailable")
	}
	settings, err := s.settingRepo.GetAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("list installed themes: %w", err)
	}
	result := &ThemeList{Themes: make([]*theme.Package, 0)}
	activeID := settings[SettingKeyActiveTheme]
	for key, raw := range settings {
		if !strings.HasPrefix(key, SettingKeyThemePackagePrefix) {
			continue
		}
		id := strings.TrimPrefix(key, SettingKeyThemePackagePrefix)
		pkg, err := decodeStoredTheme(id, raw)
		if err != nil {
			continue
		}
		result.Themes = append(result.Themes, pkg)
		if id == activeID {
			result.ActiveTheme = pkg
		}
	}
	sort.Slice(result.Themes, func(i, j int) bool {
		left, right := result.Themes[i], result.Themes[j]
		leftTime, _ := time.Parse(time.RFC3339Nano, left.InstalledAt)
		rightTime, _ := time.Parse(time.RFC3339Nano, right.InstalledAt)
		if !leftTime.Equal(rightTime) {
			return leftTime.After(rightTime)
		}
		return left.ID < right.ID
	})
	return result, nil
}

// GetActiveTheme falls back to the built-in theme for an absent or corrupt
// reference. A database failure is returned rather than hidden as a reset.
func (s *SettingService) GetActiveTheme(ctx context.Context) (*theme.Package, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("theme settings repository is unavailable")
	}
	id, err := s.settingRepo.GetValue(ctx, SettingKeyActiveTheme)
	if errors.Is(err, ErrSettingNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read active theme: %w", err)
	}
	if id == "" || !theme.ValidPackageID(id) {
		return nil, nil
	}
	pkg, err := s.loadTheme(ctx, id)
	if errors.Is(err, ErrSettingNotFound) || errors.Is(err, errInvalidStoredTheme) {
		return nil, nil
	}
	return pkg, err
}

// ActivateTheme changes one database key. Packages are never deleted, so an
// activation cannot race with deletion on another application instance.
func (s *SettingService) ActivateTheme(ctx context.Context, id string) (*theme.Package, error) {
	if s == nil || s.settingRepo == nil {
		return nil, fmt.Errorf("theme settings repository is unavailable")
	}
	var pkg *theme.Package
	if id != "" {
		if !theme.ValidPackageID(id) {
			return nil, infraerrors.BadRequest("INVALID_THEME_ID", "theme_id must be an installed theme package ID or null")
		}
		var err error
		pkg, err = s.loadTheme(ctx, id)
		if errors.Is(err, ErrSettingNotFound) {
			return nil, infraerrors.NotFound("THEME_NOT_FOUND", "theme package is not installed")
		}
		if errors.Is(err, errInvalidStoredTheme) {
			return nil, infraerrors.BadRequest("INVALID_THEME_PACKAGE", "stored theme package is invalid; upload a valid theme package")
		}
		if err != nil {
			return nil, err
		}
	}
	if err := s.settingRepo.Set(ctx, SettingKeyActiveTheme, id); err != nil {
		return nil, fmt.Errorf("activate theme: %w", err)
	}
	return pkg, nil
}

func (s *SettingService) loadTheme(ctx context.Context, id string) (*theme.Package, error) {
	raw, err := s.settingRepo.GetValue(ctx, SettingKeyThemePackagePrefix+id)
	if err != nil {
		return nil, err
	}
	return decodeStoredTheme(id, raw)
}

func decodeStoredTheme(id, raw string) (*theme.Package, error) {
	if !theme.ValidPackageID(id) || len(raw) > theme.MaxManifestBytes+1024 {
		return nil, errInvalidStoredTheme
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var stored struct {
		ID          string          `json:"id"`
		Manifest    json.RawMessage `json:"manifest"`
		InstalledAt string          `json:"installed_at"`
	}
	if err := decoder.Decode(&stored); err != nil {
		return nil, errInvalidStoredTheme
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, errInvalidStoredTheme
	}
	if stored.ID != id {
		return nil, errInvalidStoredTheme
	}
	if _, err := time.Parse(time.RFC3339Nano, stored.InstalledAt); err != nil {
		return nil, errInvalidStoredTheme
	}
	manifest, err := theme.ParseManifest(stored.Manifest)
	if err != nil {
		return nil, errInvalidStoredTheme
	}
	return &theme.Package{ID: id, Manifest: *manifest, InstalledAt: stored.InstalledAt}, nil
}
