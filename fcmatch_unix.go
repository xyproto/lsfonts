//go:build !windows

package main

import (
	"os"
	"os/exec"
	"strings"
)

// fcMatchUI returns fc-match's resolved file path for pattern, or "" on
// any failure. The family isn't verified -- any sans-serif fontconfig
// picks is good enough for the UI label strip.
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
