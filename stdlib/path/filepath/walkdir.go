package filepath

import (
	"errors"
	"os"
)

// SkipDir signals WalkDir to skip the directory whose entry fn just
// returned it for. Compared by errors.Is/== string equality (see
// runtime.hpp's operator==(Error, Error)), same as real Go's fs.SkipDir.
var SkipDir = errors.New("skip this directory")

// WalkDir walks the file tree rooted at root, calling fn for each entry.
// Bounded subset of real Go's path/filepath.WalkDir (whose signature takes
// an fs.WalkDirFunc): root itself is never passed to fn, only its
// descendants -- callers that need to filter on the root's own name check
// it themselves before calling WalkDir (see bundle.Collect, the first
// caller, which os.Stats every root up front anyway). fn returning SkipDir
// prunes that one entry -- a directory's contents when d.IsDir(), or just
// that entry when not -- real Go's "skip the rest of the containing
// directory's siblings" nuance for a non-directory SkipDir isn't
// implemented, since no caller here relies on it. Any other non-nil error
// aborts the walk immediately and is returned to the caller.
func WalkDir(root string, fn func(path string, d os.DirEntry, err error) error) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, d := range entries {
		path := Join(root, d.Name())
		cbErr := fn(path, d, nil)
		if cbErr == SkipDir {
			continue
		}
		if cbErr != nil {
			return cbErr
		}
		if d.IsDir() {
			if err := WalkDir(path, fn); err != nil {
				return err
			}
		}
	}
	return nil
}
