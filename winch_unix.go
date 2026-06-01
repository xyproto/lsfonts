//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyWinch arranges for ch to receive every SIGWINCH delivered to the
// process. On Unix this is how terminals notify foreground programs that
// the window has been resized.
func notifyWinch(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH)
}
