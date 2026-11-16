package command

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/MehulxBuilds/dora/engine"
)

type database struct {
	Name string
	Path string
}

// discover considers known snapshot extensions and WAL-backed databases.
// Symlinks and temporary files are skipped. It never creates the directory.
func discover(directory string) ([]database, error) {
	root, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("cannot read Dora directory %q: %w (run from your project root or use --path <directory>)", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("--path must be a directory: %s", root)
	}
	candidates := map[string]bool{}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".wal" {
			candidates[strings.TrimSuffix(path, filepath.Ext(path))] = true
			return nil
		}
		if ext == ".db" || ext == ".json" || ext == ".dora" {
			candidates[path] = true
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(candidates))
	for path := range candidates {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]database, 0, len(paths))
	for _, path := range paths {
		// WAL discovery must not turn a snapshot symlink into a candidate.
		if info, err := os.Lstat(path); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular()) {
			continue
		}
		if _, err := engine.InspectFile(path); err != nil {
			return nil, fmt.Errorf("inspect %s: %w", path, err)
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil, err
		}
		result = append(result, database{Name: filepath.ToSlash(relative), Path: path})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no Dora databases found in %s (expected .db, .json, .dora files or a database WAL)", root)
	}
	return result, nil
}
