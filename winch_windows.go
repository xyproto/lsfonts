//go:build windows

package main

import "os"

// notifyWinch is a no-op on platforms without SIGWINCH. The UI loop still
// recomputes geometry every frame, so very-occasional resize handling
// still works -- it just won't be live until the next keypress.
func notifyWinch(ch chan<- os.Signal) {}
