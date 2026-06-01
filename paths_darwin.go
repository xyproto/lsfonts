//go:build darwin

package main

import (
	"os"
	"path/filepath"
)

func systemFontDirs() []string {
	dirs := []string{
		"/Library/Fonts",
		"/System/Library/Fonts",
		"/System/Library/Fonts/Supplemental",
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
	}
	return dirs
}
