package main

import (
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/image/font/opentype"
)

// uiFontOnce guards lazy loading of the parsed UI font, which is shared
// across every preview row that needs a readable family/style label.
var (
	uiFontOnce   sync.Once
	uiFontParsed *opentype.Font
)

// uiFont returns a parsed sans-serif font used to render entry labels
// (family + style) so symbol/icon faces remain readable. Returns nil when
// no candidate could be found; callers then fall back to the entry's own
// face for the label, which is the previous behaviour.
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

// findUIFontPath locates a readable sans-serif font on the host. fontconfig
// is tried first because its result reflects the distro's configured default;
// a directory scan over known filenames is the fallback when fc-match is
// unavailable (typical on Windows and minimal containers).
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

	// Filename fallbacks for hosts without fontconfig. Order roughly
	// matches the patterns above so the visual style stays consistent.
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

// scanForFontName looks for any of names directly under dir or one level deep.
// One level of depth covers per-family subdirectories like /usr/share/fonts/dejavu/.
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

// fcMatchUI is provided per-platform: a real fontconfig call on Unix-likes,
// and a stub returning "" on Windows where fc-match is not part of the base
// system. Selecting at compile time avoids spawning a process that will
// never succeed on platforms without fontconfig.
