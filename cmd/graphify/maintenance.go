package main

import (
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/dobbo-ca/graphify-go/internal/cache"
	"github.com/dobbo-ca/graphify-go/internal/detect"
)

// hookMarker tags hook scripts graphify wrote, so reinstalling overwrites our
// own scripts but never clobbers a hook the user wrote.
const hookMarker = "# graphify-managed hook"

// managedGitHooks fire an incremental rebuild after history changes.
var managedGitHooks = []string{"post-commit", "post-merge", "post-checkout"}

// cmdHook handles `graphify hook <install|uninstall|status> [path]`, managing
// the git hooks that keep the graph fresh by running `graphify update` after
// commits, merges, and checkouts.
func cmdHook(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: graphify hook <install|uninstall|status> [path]")
	}
	root := "."
	if len(args) > 1 {
		root = args[1]
	}
	switch args[0] {
	case "install":
		return hookInstall(root)
	case "uninstall":
		return hookUninstall(root)
	case "status":
		return hookStatus(root)
	default:
		return fmt.Errorf("usage: graphify hook <install|uninstall|status> [path]")
	}
}

// hookInstall writes graphify's update hooks, skipping any hook a user wrote.
func hookInstall(root string) error {
	hooksDir := filepath.Join(root, ".git", "hooks")
	if fi, err := os.Stat(filepath.Dir(hooksDir)); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s has no .git directory (git worktrees and submodules are not supported by hook install)", root)
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	script := fmt.Sprintf("#!/bin/sh\n%s\nexec %q update %q >/dev/null 2>&1 || true\n", hookMarker, self, absRoot)

	var installed []string
	for _, h := range managedGitHooks {
		path := filepath.Join(hooksDir, h)
		if existing, err := os.ReadFile(path); err == nil && !strings.Contains(string(existing), hookMarker) {
			fmt.Fprintf(os.Stderr, "  warning: %s already exists and was not written by graphify — skipping\n", h)
			continue
		}
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			return err
		}
		installed = append(installed, h)
	}
	fmt.Printf("installed git hooks (%s) → %s\n", strings.Join(installed, ", "), hooksDir)
	return nil
}

// hookUninstall removes the graphify-managed hooks. The scripts are whole-file
// graphify-owned, so a present marker means we wrote the file and can delete it;
// hooks the user wrote (no marker) are left untouched.
func hookUninstall(root string) error {
	hooksDir := filepath.Join(root, ".git", "hooks")
	for _, h := range managedGitHooks {
		path := filepath.Join(hooksDir, h)
		existing, err := os.ReadFile(path)
		if err != nil {
			continue // no such hook — nothing to remove
		}
		if !strings.Contains(string(existing), hookMarker) {
			fmt.Fprintf(os.Stderr, "  warning: %s was not written by graphify — skipping\n", h)
			continue
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		fmt.Printf("removed %s\n", h)
	}
	return nil
}

// hookStatus reports, per managed hook, whether the graphify hook is installed,
// in machine-checkable output.
func hookStatus(root string) error {
	hooksDir := filepath.Join(root, ".git", "hooks")
	for _, h := range managedGitHooks {
		state := "not installed"
		if existing, err := os.ReadFile(filepath.Join(hooksDir, h)); err == nil && strings.Contains(string(existing), hookMarker) {
			state = "installed"
		}
		fmt.Printf("%s: %s\n", h, state)
	}
	return nil
}

// cmdWatch handles `graphify watch [path]`: it does one incremental update, then
// polls the tree and rebuilds whenever a source file's content changes. It is
// poll-based (no native filesystem-event dependency); Ctrl-C stops it.
func cmdWatch(root string) error {
	if err := cmdUpdate([]string{root}); err != nil {
		return err
	}
	fmt.Println("watching for changes (Ctrl-C to stop)…")

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			fmt.Println("\nstopped")
			return nil
		case <-ticker.C:
			changed, err := watchTick(root)
			if err != nil {
				fmt.Fprintln(os.Stderr, "graphify:", err)
				continue
			}
			if changed {
				if err := cmdUpdate([]string{root}); err != nil {
					fmt.Fprintln(os.Stderr, "graphify:", err)
				}
			}
		}
	}
}

// watchTick reports whether any collected file's content differs from the cache
// on disk (added, modified, or removed), without rebuilding. It is the testable
// unit of the watch loop.
func watchTick(root string) (bool, error) {
	files, err := detect.CollectFiles(root)
	if err != nil {
		return false, err
	}
	prev := cache.Load(filepath.Join(root, "graphify-out", cache.FileName))
	prevStat := cache.LoadStat(filepath.Join(root, "graphify-out", cache.StatFileName))
	seen := map[string]bool{}
	for _, f := range files {
		slash := filepath.ToSlash(f)
		seen[slash] = true
		ps, psOK := prevStat[slash]
		h, _, _, ok := cache.HashFile(filepath.Join(root, f), ps, psOK)
		if !ok {
			return true, nil // unreadable now but collected — treat as a change
		}
		e, ok := prev[slash]
		if !ok || e.Hash != h {
			return true, nil
		}
	}
	for f := range prev {
		if !seen[f] {
			return true, nil // a previously-graphed file was removed
		}
	}
	return false, nil
}
