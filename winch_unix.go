//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

// notifyWinch wires ch to SIGWINCH (terminal resize).
func notifyWinch(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGWINCH)
}
