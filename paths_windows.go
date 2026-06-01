//go:build windows

package main

import (
	"os"
	"path/filepath"
)

func systemFontDirs() []string {
	winDir := os.Getenv("WINDIR")
	if winDir == "" {
		winDir = `C:\Windows`
	}
	dirs := []string{filepath.Join(winDir, "Fonts")}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		// Per-user fonts installed without admin rights live here on
		// Windows 10+ (the "Install for me only" option in the Fonts panel).
		dirs = append(dirs,
			filepath.Join(home, "AppData", "Local", "Microsoft", "Windows", "Fonts"))
	}
	return dirs
}
