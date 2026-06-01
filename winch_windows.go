//go:build windows

package main

import "os"

// notifyWinch is a no-op: Windows has no SIGWINCH. Resize still works
// (geometry is reread each frame), just not until the next keypress.
func notifyWinch(ch chan<- os.Signal) {}
