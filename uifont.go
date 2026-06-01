package main

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font/opentype"
)

var (
	uiFontOnce   sync.Once
	uiFontParsed *opentype.Font
)

// uiFont returns a shared sans-serif used for entry labels so symbol/icon
// faces stay readable. nil means no candidate was found; callers should
// fall back to the entry's own face.
func uiFont() *opentype.Font {
	uiFontOnce.Do(func() {
		if p := findUIFontPath(); p != "" {
			if data, err := os.ReadFile(p); err == nil {
				if coll, err := opentype.ParseCollection(data); err == nil && coll.NumFonts() > 0 {
					if f, err := coll.Font(0); err == nil {
						uiFontParsed = f
					}
				}
			}
		}
	})
	return uiFontParsed
}

// findUIFontPath returns a readable sans-serif. fontconfig first (reflects
// distro defaults); directory scan as the fontconfig-less fallback.
func findUIFontPath() string {
	patterns := []string{
		"DejaVu Sans:style=Book",
		"Liberation Sans:style=Regular",
		"Cantarell:style=Regular",
		"Noto Sans:style=Regular",
		"Arial:style=Regular",
		"sans",
	}
	for _, pat := range patterns {
		if p := fcMatchUI(pat); p != "" {
			return p
		}
	}

	candidates := []string{
		"DejaVuSans.ttf",
		"LiberationSans-Regular.ttf",
		"Cantarell-Regular.otf", "Cantarell-Regular.ttf",
		"NotoSans-Regular.ttf",
		"arial.ttf", "Arial.ttf",
		"segoeui.ttf", // Windows
	}
	for _, dir := range systemFontDirs() {
		if p := scanForFontName(dir, candidates); p != "" {
			return p
		}
	}
	return ""
}

// scanForFontName checks dir and its immediate subdirectories (covers
// per-family layouts like /usr/share/fonts/dejavu/) for any of names.
func scanForFontName(dir string, names []string) string {
	for _, name := range names {
		p := filepath.Join(dir, name)
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	subs, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, sub := range subs {
		if !sub.IsDir() {
			continue
		}
		for _, name := range names {
			p := filepath.Join(dir, sub.Name(), name)
			if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
				return p
			}
		}
	}
	return ""
}

// fcMatchUI is implemented per-platform in fcmatch_*.go.
