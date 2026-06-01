//go:build windows

package main

// fcMatchUI is a stub on Windows: fontconfig is not part of the base system
// and the directory-scan fallback in findUIFontPath covers the typical
// Windows fonts (Segoe UI, Arial) directly.
func fcMatchUI(string) string { return "" }
