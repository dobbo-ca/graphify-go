//go:build !unix

package main

// handleBrokenPipe is a no-op on platforms without SIGPIPE (e.g. Windows).
func handleBrokenPipe() {}

// isBrokenPipe is always false on platforms without SIGPIPE (e.g. Windows).
func isBrokenPipe(error) bool { return false }
