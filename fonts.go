package main

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/image/font/opentype"
	"golang.org/x/image/font/sfnt"
)

// FontEntry is one previewable font face. Index is the face offset inside
// a TTC collection (0 for plain OTF/TTF).
type FontEntry struct {
	Path    string
	Index   int
	Family  string
	Style   string
	Display string // "Family — Style" or basename when names are unknown
}

// CopyName returns "Family Style" (or just "Family" for Regular). The form
// pastes into CSS, Vim guifont and Pango without manual editing.
func (e FontEntry) CopyName() string {
	if e.Style == "" || strings.EqualFold(e.Style, "Regular") {
		return e.Family
	}
	return e.Family + " " + e.Style
}

// DiscoverFonts walks the host's system font directories and returns one
// FontEntry per face, sorted by family then style. TTC collections are
// expanded so every contained face is previewable on its own.
func DiscoverFonts() []FontEntry {
	seen := make(map[string]bool)
	var entries []FontEntry
	for _, dir := range systemFontDirs() {
		_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() || !looksLikeFont(p) {
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

// expandFontFile yields one FontEntry per face in p. Unparseable files are
// silently skipped.
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

// readNames pulls family + style from the font's name table. Errors are
// swallowed: some fonts have partial or non-ASCII name records.
func readNames(f *opentype.Font) (family, style string) {
	if s, err := f.Name(nil, sfnt.NameIDFamily); err == nil {
		family = strings.TrimSpace(s)
	}
	if s, err := f.Name(nil, sfnt.NameIDSubfamily); err == nil {
		style = strings.TrimSpace(s)
	}
	return
}

func formatDisplay(family, style string) string {
	if style == "" || strings.EqualFold(style, "Regular") {
		return family
	}
	return family + " — " + style
}

func baseNameWithoutExt(p string) string {
	b := filepath.Base(p)
	if i := strings.LastIndexByte(b, '.'); i >= 0 {
		b = b[:i]
	}
	return b
}

func looksLikeFont(p string) bool {
	lower := strings.ToLower(p)
	return strings.HasSuffix(lower, ".otf") ||
		strings.HasSuffix(lower, ".ttf") ||
		strings.HasSuffix(lower, ".ttc")
}

// systemFontDirs is implemented per-platform in paths_*.go.
