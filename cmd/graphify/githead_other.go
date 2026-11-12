//go:build !unix

package main

import "os"

// No owner check here: always ask git.
func ownedByMe(string) bool { return false }

// Devices unknown: walk on, any repo found falls back.
func sameDevice(a, b os.FileInfo) bool { return true }
