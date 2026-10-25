/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package dashboard

import (
	"bytes"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"
)

func readEmbedded(t *testing.T, name string) string {
	t.Helper()
	b, err := webFiles.ReadFile("web/" + name)
	if err != nil {
		t.Fatalf("reading embedded %s: %v", name, err)
	}
	return string(b)
}

// The embedded brand files are copies (go:embed cannot reach assets/brand).
// This keeps them from drifting. Refresh with `make dashboard-assets`.
func TestBrandCopiesMatchAssets(t *testing.T) {
	for _, name := range []string{"favicon.svg", "favicon.ico", "apple-touch-icon.png", "logo.svg"} {
		want, err := os.ReadFile("../../assets/brand/" + name)
		if err != nil {
			t.Fatalf("reading assets/brand/%s: %v", name, err)
		}
		got, err := webFiles.ReadFile("web/" + name)
		if err != nil {
			t.Fatalf("reading embedded %s: %v", name, err)
		}
		if !bytes.Equal(want, got) {
			t.Errorf("web/%s differs from assets/brand/%s; run `make dashboard-assets`", name, name)
		}
	}
}

// The page must work in an offline cluster and under the strict CSP, so no
// remote URLs, no inline code, and no way to turn text into HTML.
func TestPageIsOfflineAndHasNoInlineCode(t *testing.T) {
	svgNamespace := "http://www.w3.org/2000/svg"
	forbiddenInJS := []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "new Function"}
	inlineScript := regexp.MustCompile(`(?is)<script\b[^>]*>\s*[^<\s]`)
	inlineStyleAttr := regexp.MustCompile(`(?i)\sstyle\s*=`)
	eventHandlerAttr := regexp.MustCompile(`(?i)\son[a-z]+\s*=`)

	err := fs.WalkDir(webFiles, "web", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		ext := path.Ext(p)
		if ext != ".html" && ext != ".css" && ext != ".js" {
			return nil
		}
		b, readErr := webFiles.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		text := strings.ReplaceAll(string(b), svgNamespace, "")
		if strings.Contains(text, "http://") || strings.Contains(text, "https://") || strings.Contains(text, "//cdn") {
			t.Errorf("%s contains a remote URL", p)
		}
		switch ext {
		case ".js":
			for _, word := range forbiddenInJS {
				if strings.Contains(text, word) {
					t.Errorf("%s uses %s", p, word)
				}
			}
		case ".html":
			if inlineScript.MatchString(text) {
				t.Errorf("%s has an inline script", p)
			}
			if inlineStyleAttr.MatchString(text) {
				t.Errorf("%s has a style attribute", p)
			}
			if eventHandlerAttr.MatchString(text) {
				t.Errorf("%s has an inline event handler", p)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Everything index.html and app.css point at must be embedded. A missing font
// means `make dashboard-fonts` was not run (or its result was not committed).
func TestReferencedFilesExist(t *testing.T) {
	index := readEmbedded(t, "index.html")
	ref := regexp.MustCompile(`(?:href|src)="(/[^"#?]*)"`)
	for _, m := range ref.FindAllStringSubmatch(index, -1) {
		if _, err := fs.Stat(webFiles, "web"+m[1]); err != nil {
			t.Errorf("index.html references %s, which is not embedded", m[1])
		}
	}

	css := readEmbedded(t, "app.css")
	font := regexp.MustCompile(`url\("(/fonts/[^"]+)"\)`)
	fonts := font.FindAllStringSubmatch(css, -1)
	if len(fonts) == 0 {
		t.Fatal("app.css declares no fonts")
	}
	for _, m := range fonts {
		if _, err := fs.Stat(webFiles, "web"+m[1]); err != nil {
			t.Errorf("app.css needs %s, which is not embedded: run `make dashboard-fonts` and commit the files", m[1])
		}
	}
	if _, err := fs.Stat(webFiles, "web/fonts/LICENSE-IBM-Plex.txt"); err != nil {
		t.Errorf("the IBM Plex licence text is missing: run `make dashboard-fonts` and commit the files")
	}

	jsImport := regexp.MustCompile(`from '(\./[^']+)'`)
	entries, err := fs.ReadDir(webFiles, "web/js")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		src := readEmbedded(t, "js/"+e.Name())
		for _, m := range jsImport.FindAllStringSubmatch(src, -1) {
			if _, err := fs.Stat(webFiles, "web/js/"+strings.TrimPrefix(m[1], "./")); err != nil {
				t.Errorf("js/%s imports %s, which is not embedded", e.Name(), m[1])
			}
		}
	}
}
