package web

import (
	"encoding/json"
	"io/fs"
	"strings"
	"testing"
)

func TestFS_ContainsExpectedAssets(t *testing.T) {
	wantFiles := []string{
		"index.html",
		"styles.css",
		"app.js",
		"strings.en.json",
		"favicon.svg",
	}

	for _, name := range wantFiles {
		t.Run(name, func(t *testing.T) {
			data, err := fs.ReadFile(FS, name)
			if err != nil {
				t.Fatalf("reading %s from embedded FS: %v", name, err)
			}
			if len(data) == 0 {
				t.Errorf("%s is embedded but empty", name)
			}
		})
	}
}

func TestFS_StringsIsValidJSON(t *testing.T) {
	data, err := fs.ReadFile(FS, "strings.en.json")
	if err != nil {
		t.Fatalf("reading strings.en.json: %v", err)
	}

	var strs map[string]string
	if err := json.Unmarshal(data, &strs); err != nil {
		t.Fatalf("strings.en.json is not a valid string map: %v", err)
	}

	// Keys the UI depends on must all be present.
	wantKeys := []string{
		"app.title", "app.tagline",
		"drop.input.title", "drop.input.hint",
		"drop.tiles.title", "drop.tiles.hint",
		"control.gridSize", "control.generate", "control.generating",
		"result.title", "result.download",
		"help.title", "help.body", "help.close",
		"error.noInput", "error.noTiles",
	}
	for _, key := range wantKeys {
		if strs[key] == "" {
			t.Errorf("strings.en.json is missing a value for %q", key)
		}
	}
}

func TestIndexHTML_ReferencesAssets(t *testing.T) {
	data, err := fs.ReadFile(FS, "index.html")
	if err != nil {
		t.Fatalf("reading index.html: %v", err)
	}
	html := string(data)

	for _, ref := range []string{"styles.css", "app.js"} {
		if !strings.Contains(html, ref) {
			t.Errorf("index.html does not reference %s", ref)
		}
	}
}
