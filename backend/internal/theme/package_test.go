package theme

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validManifest = `{"schema_version":1,"id":"ocean","name":"Ocean","version":"1.2.3-beta.1+build.2","tokens":{"colors":{"primary":{"500":"#1234aB"}},"font":"system","radius_scale":1.25}}`

type archiveEntry struct {
	name, body string
	mode       os.FileMode
}

func makeArchive(t *testing.T, entries ...archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Store}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		f, err := w.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestParseAcceptsDeclarativePackageAndOptionalDocuments(t *testing.T) {
	archive := makeArchive(t, archiveEntry{"theme.json", validManifest, 0}, archiveEntry{"README.md", "# Ocean", 0}, archiveEntry{"LICENSE", "MIT", 0})
	m, err := Parse(archive)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != "ocean" || m.Tokens.Colors.Primary["500"] != "#1234aB" {
		t.Fatalf("unexpected manifest: %#v", m)
	}
	if !ValidPackageID(PackageID(archive)) {
		t.Fatal("invalid package hash")
	}
}

// Exercise the actual downloadable artifact, so changes to the example packer
// or example source cannot silently diverge from the backend schema contract.
func TestParsePublishedOceanExample(t *testing.T) {
	frontend := filepath.Join("..", "..", "..", "frontend")
	if info, err := os.Stat(frontend); os.IsNotExist(err) {
		t.Skip("frontend directory is absent in this backend-only checkout")
	} else if err != nil {
		t.Fatal(err)
	} else if !info.IsDir() {
		t.Fatal("frontend path is not a directory")
	}
	archive, err := os.ReadFile(filepath.Join(frontend, "public", "themes", "examples", "ocean.s2theme"))
	if err != nil {
		t.Fatalf("read published Ocean theme package: %v", err)
	}
	manifest, err := Parse(archive)
	if err != nil {
		t.Fatalf("published Ocean theme does not satisfy schema %d: %v", SchemaVersion, err)
	}
	if manifest.SchemaVersion != SchemaVersion || manifest.ID != "ocean" || manifest.Name != "Ocean" || manifest.Version != "1.0.0" {
		t.Fatalf("unexpected example identity: %#v", manifest)
	}
	if manifest.Tokens.Font == nil || *manifest.Tokens.Font != "system" || manifest.Tokens.RadiusScale == nil || *manifest.Tokens.RadiusScale != 1.25 {
		t.Fatalf("unexpected Ocean font or radius tokens: %#v", manifest.Tokens)
	}
	if manifest.Tokens.Colors == nil || len(manifest.Tokens.Colors.Primary) != 11 || manifest.Tokens.Colors.Primary["500"] != "#3b82f6" {
		t.Fatalf("Ocean example must publish the complete blue primary palette: %#v", manifest.Tokens.Colors)
	}
	if !ValidPackageID(PackageID(archive)) {
		t.Fatal("published example has an invalid content ID")
	}
	// The downloadable ZIP and editable source must describe the same manifest.
	source, err := os.ReadFile(filepath.Join(frontend, "themes", "examples", "ocean", "theme.json"))
	if err != nil {
		t.Fatalf("read editable Ocean theme manifest: %v", err)
	}
	sourceManifest, err := ParseManifest(source)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := json.Marshal(sourceManifest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatal("published Ocean theme archive differs from its editable source manifest")
	}
}

func TestParseRejectsUnsafeOrAmbiguousArchives(t *testing.T) {
	tests := map[string][]archiveEntry{
		"missing manifest": {{"README.md", "hello", 0}},
		"path traversal":   {{"../theme.json", validManifest, 0}},
		"absolute path":    {{"/theme.json", validManifest, 0}},
		"nested manifest":  {{"ocean/theme.json", validManifest, 0}},
		"backslash path":   {{`..\theme.json`, validManifest, 0}},
		"symlink":          {{"theme.json", validManifest, os.ModeSymlink | 0777}},
		"directory":        {{"theme.json", validManifest, os.ModeDir | 0755}},
		"duplicate":        {{"theme.json", validManifest, 0}, {"theme.json", validManifest, 0}},
		"script":           {{"theme.json", validManifest, 0}, {"main.js", "alert(1)", 0}},
		"css":              {{"theme.json", validManifest, 0}, {"theme.css", "body{}", 0}},
		"asset":            {{"theme.json", validManifest, 0}, {"logo.png", "png", 0}},
		"too many entries": {{"theme.json", validManifest, 0}, {"README.md", "a", 0}, {"LICENSE", "b", 0}, {"extra", "c", 0}},
		"large manifest":   {{"theme.json", strings.Repeat(" ", MaxManifestBytes+1), 0}},
		"large document":   {{"theme.json", validManifest, 0}, {"README.md", strings.Repeat(" ", MaxDocumentBytes+1), 0}},
	}
	for name, entries := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := Parse(makeArchive(t, entries...)); err == nil {
				t.Fatal("expected archive rejection")
			}
		})
	}
	for _, archive := range [][]byte{nil, []byte("invalid"), make([]byte, MaxPackageBytes+1)} {
		if _, err := Parse(archive); err == nil {
			t.Fatal("expected invalid archive rejection")
		}
	}
}

func TestParseRejectsCRCAndSizeCorruptionIncludingDocuments(t *testing.T) {
	for _, name := range []string{"theme.json", "README.md"} {
		t.Run(name, func(t *testing.T) {
			archive := makeArchive(t, archiveEntry{"theme.json", validManifest, 0}, archiveEntry{"README.md", "documentation", 0})
			reader, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
			if err != nil {
				t.Fatal(err)
			}
			for _, file := range reader.File {
				if file.Name == name {
					offset, err := file.DataOffset()
					if err != nil {
						t.Fatal(err)
					}
					archive[offset] ^= 1
				}
			}
			if _, err := Parse(archive); err == nil {
				t.Fatal("expected CRC rejection")
			}
		})
	}
	// A forged central-directory size must not bypass actual read limits.
	archive := makeArchive(t, archiveEntry{"theme.json", validManifest, 0})
	offset := bytes.Index(archive, []byte{'P', 'K', 1, 2})
	if offset < 0 {
		t.Fatal("missing central directory")
	}
	binary.LittleEndian.PutUint32(archive[offset+24:], MaxManifestBytes+1)
	if _, err := Parse(archive); err == nil {
		t.Fatal("expected oversized ZIP header rejection")
	}
}

func TestParseManifestStrictSchema(t *testing.T) {
	tests := map[string]string{
		"unknown field":       strings.Replace(validManifest, `"name":"Ocean"`, `"name":"Ocean","script":"alert(1)"`, 1),
		"wrong case":          strings.Replace(validManifest, `"name"`, `"Name"`, 1),
		"unknown tokens":      strings.Replace(validManifest, `"font":"system"`, `"css":"body{}"`, 1),
		"unknown palette":     strings.Replace(validManifest, `"primary"`, `"background"`, 1),
		"unknown shade":       strings.Replace(validManifest, `"500"`, `"501"`, 1),
		"css color":           strings.Replace(validManifest, `#1234aB`, `red;display:none`, 1),
		"short color":         strings.Replace(validManifest, `#1234aB`, `#fff`, 1),
		"duplicate field":     strings.Replace(validManifest, `"name":"Ocean"`, `"name":"Ocean","name":"Other"`, 1),
		"duplicate shade":     strings.Replace(validManifest, `"500":"#1234aB"`, `"500":"#1234aB","500":"#ffffff"`, 1),
		"null token":          strings.Replace(validManifest, `"system"`, `null`, 1),
		"wrong type":          strings.Replace(validManifest, `"radius_scale":1.25`, `"radius_scale":"1.25"`, 1),
		"radius small":        strings.Replace(validManifest, `1.25`, `0.24`, 1),
		"radius large":        strings.Replace(validManifest, `1.25`, `2.01`, 1),
		"remote font":         strings.Replace(validManifest, `"system"`, `"https://x/font.woff"`, 1),
		"unsupported schema":  strings.Replace(validManifest, `"schema_version":1`, `"schema_version":2`, 1),
		"slug":                strings.Replace(validManifest, `"id":"ocean"`, `"id":"../ocean"`, 1),
		"metadata control":    strings.Replace(validManifest, `"name":"Ocean"`, `"name":"Ocean\u0000"`, 1),
		"metadata whitespace": strings.Replace(validManifest, `"name":"Ocean"`, `"name":" Ocean"`, 1),
		"metadata too long":   strings.Replace(validManifest, `"name":"Ocean"`, `"name":"`+strings.Repeat("名", 81)+`"`, 1),
		"version":             strings.Replace(validManifest, `1.2.3-beta.1+build.2`, `01.2.3`, 1),
		"version prerelease":  strings.Replace(validManifest, `1.2.3-beta.1+build.2`, `1.2.3-beta.01`, 1),
		"trailing JSON":       validManifest + `{}`,
		"empty tokens":        `{"schema_version":1,"id":"plain","name":"Plain","version":"1.0.0","tokens":{}}`,
		"empty palette":       `{"schema_version":1,"id":"plain","name":"Plain","version":"1.0.0","tokens":{"colors":{"primary":{}}}}`,
		"empty colors":        `{"schema_version":1,"id":"plain","name":"Plain","version":"1.0.0","tokens":{"colors":{}}}`,
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseManifest([]byte(raw)); err == nil {
				t.Fatalf("expected manifest rejection: %s", raw)
			}
		})
	}
	for _, tokens := range []string{`{"font":"serif"}`, `{"font":"mono"}`, `{"radius_scale":0.25}`, `{"radius_scale":2}`, `{"colors":{"dark":{"950":"#000000"}}}`} {
		raw := `{"schema_version":1,"id":"minimal","name":"Minimal","version":"0.0.1","tokens":` + tokens + `}`
		if _, err := ParseManifest([]byte(raw)); err != nil {
			t.Fatalf("valid partial tokens %s: %v", tokens, err)
		}
	}
}

func FuzzParse(f *testing.F) {
	f.Add([]byte("invalid zip"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) { _, _ = Parse(data) })
}
