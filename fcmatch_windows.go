//go:build windows

package main

// fcMatchUI is a stub: Windows has no fontconfig; findUIFontPath's
// directory-scan fallback covers Segoe UI / Arial directly.
func fcMatchUI(string) string { return "" }
