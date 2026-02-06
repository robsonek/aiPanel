package aipanel

import (
	"io/fs"
	"testing"
)

func TestFrontendFS_ContainsIndexHTML(t *testing.T) {
	f, err := fs.ReadFile(FrontendFS, "web/dist/index.html")
	if err != nil {
		t.Fatalf("expected web/dist/index.html in embedded FS: %v", err)
	}
	if len(f) == 0 {
		t.Fatal("index.html is empty")
	}
}

func TestFrontendFS_ContainsAssets(t *testing.T) {
	entries, err := fs.ReadDir(FrontendFS, "web/dist/assets")
	if err != nil {
		t.Fatalf("expected web/dist/assets/ directory: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("assets directory is empty")
	}

	var hasJS, hasCSS bool
	for _, e := range entries {
		name := e.Name()
		if len(name) > 3 && name[len(name)-3:] == ".js" {
			hasJS = true
		}
		if len(name) > 4 && name[len(name)-4:] == ".css" {
			hasCSS = true
		}
	}
	if !hasJS {
		t.Error("no .js file found in assets")
	}
	if !hasCSS {
		t.Error("no .css file found in assets")
	}
}
