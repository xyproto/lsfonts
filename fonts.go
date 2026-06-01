package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// FontEntry represents one installed font file that can be previewed.
// Index is the entry's offset inside a TTC collection (0 for plain OTF/TTF).
type FontEntry struct {
	Path    string
	Index   int
	Family  string
	Style   string
	Display string // "Family — Style" or just basename when the names are unknown
}

// DiscoverFonts walks the standard font directories for the host OS and
// returns a sorted slice of FontEntry. Each entry inside a TTC collection
// is yielded separately so users can preview every face.
func DiscoverFonts() []FontEntry {
	seen := make(map[string]bool)
	var entries []FontEntry
	for _, dir := range systemFontDirs() {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if !looksLikeFont(p) {
				return nil
			}
			real, err := filepath.EvalSymlinks(p)
			if err != nil {
				real = p
			}
			if seen[real] {
				return nil
			}
			seen[real] = true
			entries = append(entries, expandFontFile(p)...)
			return nil
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Family != entries[j].Family {
			return strings.ToLower(entries[i].Family) < strings.ToLower(entries[j].Family)
		}
		return strings.ToLower(entries[i].Style) < strings.ToLower(entries[j].Style)
	})
	return entries
}

// expandFontFile reads p and returns one FontEntry per face. For a TTC
// collection that is one entry per contained font; for plain OTF/TTF it is
// a single entry. A file that cannot be parsed is skipped silently.
func expandFontFile(p string) []FontEntry {
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	coll, err := opentype.ParseCollection(data)
	if err != nil {
		return nil
	}
	n := coll.NumFonts()
	if n == 0 {
		return nil
	}
	out := make([]FontEntry, 0, n)
	for i := range n {
		f, err := coll.Font(i)
		if err != nil || f == nil {
			continue
		}
		family, style := readNames(f)
		if family == "" {
			family = baseNameWithoutExt(p)
		}
		out = append(out, FontEntry{
			Path:    p,
			Index:   i,
			Family:  family,
			Style:   style,
			Display: formatDisplay(family, style),
		})
	}
	return out
}

// readNames extracts the family and style strings from a parsed font. Errors
// are swallowed because some fonts have partial or non-ASCII name tables.
func readNames(f *opentype.Font) (family, style string) {
	if s, err := f.Name(nil, sfnt.NameIDFamily); err == nil {
		family = strings.TrimSpace(s)
	}
	if s, err := f.Name(nil, sfnt.NameIDSubfamily); err == nil {
		style = strings.TrimSpace(s)
	}
	return
}

// formatDisplay joins family and style with an em dash, falling back to just
// the family when the style is missing or generic.
func formatDisplay(family, style string) string {
	if style == "" || strings.EqualFold(style, "Regular") {
		return family
	}
	return family + " — " + style
}

// baseNameWithoutExt returns the base filename of p with its extension
// stripped. Used as a last-resort family name.
func baseNameWithoutExt(p string) string {
	b := filepath.Base(p)
	if i := strings.LastIndexByte(b, '.'); i >= 0 {
		b = b[:i]
	}
	return b
}

// looksLikeFont returns true when p has a recognized font extension. The
// file is not opened here; expandFontFile does the actual parse and rejects
// unreadable or unparseable files.
func looksLikeFont(p string) bool {
	lower := strings.ToLower(p)
	return strings.HasSuffix(lower, ".otf") ||
		strings.HasSuffix(lower, ".ttf") ||
		strings.HasSuffix(lower, ".ttc")
}

// systemFontDirs is provided per-platform in paths_*.go. The list is the
// canonical font search path that fontconfig and standard packagers use
// for the host OS, in the order they should be probed.
