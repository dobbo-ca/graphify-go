//go:build unix

package main

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
)

// handleBrokenPipe makes graphify exit 0 when a downstream reader closes the
// pipe early (e.g. `graphify query . | head`). Go's default terminates the
// process with SIGPIPE (exit 141) on a broken stdout/stderr, which CI wrappers
// and agent harnesses misread as a command failure (#1807).
func handleBrokenPipe() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGPIPE)
	go func() {
		<-c
		os.Exit(0)
	}()
}

// isBrokenPipe reports whether err is an EPIPE from writing to a closed pipe.
// Commands that PROPAGATE their write error (e.g. `diff --json`) use this to exit
// 0 synchronously, rather than racing the async SIGPIPE handler against exit 1.
func isBrokenPipe(err error) bool {
	return errors.Is(err, syscall.EPIPE)
}
