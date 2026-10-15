// Package theme defines the versioned, declarative theme format. It has no
// dependency on the web server, database, or executable plugin runtime.
package theme

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	SchemaVersion        = 1
	MaxPackageBytes      = 256 << 10
	MaxManifestBytes     = 32 << 10
	MaxDocumentBytes     = 32 << 10
	MaxUncompressedBytes = MaxManifestBytes + 2*MaxDocumentBytes
)

type Manifest struct {
	SchemaVersion int    `json:"schema_version"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Version       string `json:"version"`
	Description   string `json:"description,omitempty"`
	Author        string `json:"author,omitempty"`
	Tokens        Tokens `json:"tokens"`
}

type Tokens struct {
	Colors      *Colors  `json:"colors,omitempty"`
	Font        *string  `json:"font,omitempty"`
	RadiusScale *float64 `json:"radius_scale,omitempty"`
}

type Colors struct {
	Primary map[string]string `json:"primary,omitempty"`
	Gray    map[string]string `json:"gray,omitempty"`
	Dark    map[string]string `json:"dark,omitempty"`
	Accent  map[string]string `json:"accent,omitempty"`
}

// Package is an installed immutable manifest, addressed by the SHA-256 of its
// original archive. Installation never writes archive entries to disk.
type Package struct {
	ID          string   `json:"id"`
	Manifest    Manifest `json:"manifest"`
	InstalledAt string   `json:"installed_at"`
}

var (
	slugPattern    = regexp.MustCompile(`^[a-z][a-z0-9]*(?:-[a-z0-9]+)*$`)
	hashPattern    = regexp.MustCompile(`^[0-9a-f]{64}$`)
	colorPattern   = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`)
)

func ValidPackageID(id string) bool { return hashPattern.MatchString(id) }

func PackageID(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Parse checks both ZIP structure and every entry's CRC, including optional
// documents. An allowlist prevents paths, links, executable code, and assets.
func Parse(data []byte) (*Manifest, error) {
	if len(data) == 0 || len(data) > MaxPackageBytes {
		return nil, fmt.Errorf("theme archive must be between 1 and %d bytes", MaxPackageBytes)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("invalid theme ZIP: %w", err)
	}
	if len(zr.File) < 1 || len(zr.File) > 3 {
		return nil, fmt.Errorf("theme archive must contain theme.json and at most README.md and LICENSE")
	}
	seen := make(map[string]bool)
	var manifest []byte
	var total uint64
	for _, entry := range zr.File {
		limit := MaxDocumentBytes
		switch entry.Name {
		case "theme.json":
			limit = MaxManifestBytes
		case "README.md", "LICENSE":
		default:
			return nil, fmt.Errorf("unsupported theme archive entry %q", entry.Name)
		}
		if seen[entry.Name] {
			return nil, fmt.Errorf("duplicate theme archive entry %q", entry.Name)
		}
		seen[entry.Name] = true
		if !entry.Mode().IsRegular() {
			return nil, fmt.Errorf("theme archive entries must be regular files")
		}
		if entry.Flags&1 != 0 {
			return nil, fmt.Errorf("encrypted theme archives are not supported")
		}
		if entry.UncompressedSize64 > uint64(limit) {
			return nil, fmt.Errorf("theme archive entry %q exceeds %d bytes", entry.Name, limit)
		}
		total += entry.UncompressedSize64
		if total > MaxUncompressedBytes {
			return nil, fmt.Errorf("theme archive exceeds uncompressed size limit")
		}
		r, err := entry.Open()
		if err != nil {
			return nil, fmt.Errorf("read theme archive: %w", err)
		}
		body, readErr := io.ReadAll(io.LimitReader(r, int64(limit)+1))
		closeErr := r.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read theme archive entry %q: %w", entry.Name, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("close theme archive entry: %w", closeErr)
		}
		if len(body) > limit || uint64(len(body)) != entry.UncompressedSize64 {
			return nil, fmt.Errorf("invalid theme archive entry size")
		}
		if entry.Name == "theme.json" {
			manifest = body
		}
	}
	if manifest == nil {
		return nil, fmt.Errorf("theme.json is required at the archive root")
	}
	return ParseManifest(manifest)
}

func ParseManifest(data []byte) (*Manifest, error) {
	if len(data) == 0 || len(data) > MaxManifestBytes || !utf8.Valid(data) {
		return nil, fmt.Errorf("theme.json must be valid UTF-8 and at most %d bytes", MaxManifestBytes)
	}
	// encoding/json otherwise accepts duplicate keys and explicit nulls, both
	// of which make a versioned contract ambiguous across implementations.
	scanner := json.NewDecoder(bytes.NewReader(data))
	scanner.UseNumber()
	if err := scanValue(scanner, 0); err != nil {
		return nil, fmt.Errorf("invalid theme manifest: %w", err)
	}
	if _, err := scanner.Token(); err != io.EOF {
		return nil, fmt.Errorf("theme.json must contain one JSON object")
	}
	if err := validateKeys(data, []string{"schema_version", "id", "name", "version", "description", "author", "tokens"}, "manifest"); err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("theme.json must contain one JSON object")
	}
	if raw, ok := fields["tokens"]; ok {
		if err := validateKeys(raw, []string{"colors", "font", "radius_scale"}, "tokens"); err != nil {
			return nil, err
		}
		var tokenFields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &tokenFields); err != nil {
			return nil, fmt.Errorf("tokens must be a JSON object")
		}
		if colors, ok := tokenFields["colors"]; ok {
			if err := validateKeys(colors, []string{"primary", "gray", "dark", "accent"}, "colors"); err != nil {
				return nil, err
			}
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("invalid theme manifest: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return nil, err
	}
	return &manifest, nil
}

func validateKeys(data []byte, allowed []string, label string) error {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(data, &values); err != nil || values == nil {
		return fmt.Errorf("%s must be a JSON object", label)
	}
	for key := range values {
		found := false
		for _, name := range allowed {
			if key == name {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("unknown %s field %q", label, key)
		}
	}
	return nil
}

func scanValue(d *json.Decoder, depth int) error {
	if depth > 8 {
		return fmt.Errorf("JSON nesting is too deep")
	}
	t, err := d.Token()
	if err != nil {
		return err
	}
	if t == nil {
		return fmt.Errorf("null is not a theme token")
	}
	delim, ok := t.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			keyToken, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return fmt.Errorf("invalid JSON object key")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = true
			if err := scanValue(d, depth+1); err != nil {
				return err
			}
		}
	case '[':
		for d.More() {
			if err := scanValue(d, depth+1); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter")
	}
	_, err = d.Token()
	return err
}

func ValidateManifest(m Manifest) error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported theme schema_version; expected %d", SchemaVersion)
	}
	if len(m.ID) > 64 || !slugPattern.MatchString(m.ID) {
		return fmt.Errorf("theme id must be a lowercase slug of at most 64 characters")
	}
	if !validText(m.Name, 80, true) {
		return fmt.Errorf("theme name must contain 1–80 printable characters")
	}
	if !validText(m.Description, 500, false) || !validText(m.Author, 120, false) {
		return fmt.Errorf("theme description or author is invalid or too long")
	}
	match := versionPattern.FindStringSubmatch(m.Version)
	if len(m.Version) > 64 || match == nil {
		return fmt.Errorf("theme version must be a semantic version of at most 64 characters")
	}
	for _, part := range strings.Split(match[4], ".") {
		if len(part) > 1 && part[0] == '0' && strings.Trim(part, "0123456789") == "" {
			return fmt.Errorf("numeric prerelease identifiers cannot have leading zeros")
		}
	}
	count := 0
	if m.Tokens.Colors != nil {
		paletteCount := 0
		for _, shades := range []map[string]string{m.Tokens.Colors.Primary, m.Tokens.Colors.Gray, m.Tokens.Colors.Dark, m.Tokens.Colors.Accent} {
			if shades == nil {
				continue
			}
			if len(shades) == 0 {
				return fmt.Errorf("color palettes must contain at least one shade")
			}
			for shade, color := range shades {
				switch shade {
				case "50", "100", "200", "300", "400", "500", "600", "700", "800", "900", "950":
				default:
					return fmt.Errorf("unsupported color shade %q", shade)
				}
				if !colorPattern.MatchString(color) {
					return fmt.Errorf("color values must use #RRGGBB")
				}
			}
			paletteCount++
		}
		if paletteCount == 0 {
			return fmt.Errorf("colors must contain at least one palette")
		}
		count++
	}
	if m.Tokens.Font != nil {
		switch *m.Tokens.Font {
		case "system", "serif", "mono":
		default:
			return fmt.Errorf("font must be system, serif, or mono")
		}
		count++
	}
	if m.Tokens.RadiusScale != nil {
		radius := *m.Tokens.RadiusScale
		if math.IsNaN(radius) || math.IsInf(radius, 0) || radius < 0.25 || radius > 2 {
			return fmt.Errorf("radius_scale must be between 0.25 and 2")
		}
		count++
	}
	if count == 0 {
		return fmt.Errorf("theme tokens must contain at least one customization")
	}
	return nil
}

func validText(value string, max int, required bool) bool {
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > max || strings.TrimSpace(value) != value || required && value == "" {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	return true
}
