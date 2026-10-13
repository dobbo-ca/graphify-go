// Package fsutil holds small filesystem helpers shared across packages.
package fsutil

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
)

// WriteFileAtomic writes data to path via a temp file in the same directory
// followed by a rename, so a concurrent reader sees either the old file or the
// complete new one, never a truncated write.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	return writeAtomic(path, perm, func(f *os.File) error {
		_, err := f.Write(data)
		return err
	})
}

// WriteAtomic is WriteFileAtomic for content produced incrementally: write
// receives a buffered writer over the temp file, so a large document never has
// to exist in memory as one slice.
func WriteAtomic(path string, perm os.FileMode, write func(w io.Writer) error) error {
	return writeAtomic(path, perm, func(f *os.File) error {
		bw := bufio.NewWriterSize(f, 1<<20)
		if err := write(bw); err != nil {
			return err
		}
		return bw.Flush()
	})
}

func writeAtomic(path string, perm os.FileMode, write func(f *os.File) error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".gfy-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() {
		f.Close()
		os.Remove(tmp)
	}()
	if err := write(f); err != nil {
		return err
	}
	if err := f.Chmod(perm); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
