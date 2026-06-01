//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
)

// fcMatchUI runs fc-match for pattern and returns the resolved font file
// path. Returns "" when fc-match is missing, errors, or yields a non-
// existent path. Unlike Orbiton's variant we don't verify the returned
// family -- any sans-serif fontconfig resolves to is good enough for the
// UI label strip.
func fcMatchUI(pattern string) string {
	out, err := exec.Command("fc-match", "--format=%{file}", pattern).Output()
	if err != nil {
		return ""
	}
	p := strings.TrimSpace(string(out))
	if p == "" {
		return ""
	}
	if fi, err := os.Stat(p); err != nil || fi.IsDir() {
		return ""
	}
	return p
}
