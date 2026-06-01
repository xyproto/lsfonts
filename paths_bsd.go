//go:build freebsd || openbsd || netbsd || dragonfly

package main

import (
	"os"
	"path/filepath"
)

func systemFontDirs() []string {
	dirs := []string{
		"/usr/local/share/fonts",
		"/usr/X11R7/lib/X11/fonts",
		"/usr/pkg/share/fonts",
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		dirs = append(dirs,
			filepath.Join(home, ".local", "share", "fonts"),
			filepath.Join(home, ".fonts"))
	}
	return dirs
}
