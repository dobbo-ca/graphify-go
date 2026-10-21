//go:build !unix

package main

import "os"

// No owner check here: always ask git.
func ownedByMe(string) bool { return false }

func sameDevice(a, b os.FileInfo) bool { return false }
