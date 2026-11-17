package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/jose00735/korokmote/daemon/internal/proto"
)

// Files lists directories inside a fixed set of roots.
type Files struct {
	roots []string
}

// NewFiles builds a file browser limited to roots. When roots is empty, the
// user's home folder and all drives are used.
func NewFiles(roots []string, shell Shell) *Files {
	var clean []string
	for _, r := range roots {
		if abs, err := filepath.Abs(r); err == nil {
			clean = append(clean, abs)
		}
	}
	if len(clean) == 0 {
		if home, err := os.UserHomeDir(); err == nil {
			clean = append(clean, home)
		}
		if shell != nil {
			clean = append(clean, shell.Drives()...)
		}
	}
	return &Files{roots: clean}
}

// Roots returns the configured roots as entries.
func (f *Files) Roots() []proto.FsEntry {
	out := make([]proto.FsEntry, 0, len(f.roots))
	for _, r := range f.roots {
		name := filepath.Base(r)
		if name == "" || name == "." || name == string(filepath.Separator) || strings.HasSuffix(r, ":\\") {
			name = r
		}
		out = append(out, proto.FsEntry{Name: name, Path: r, Dir: true})
	}
	return out
}

func samePrefix(path, root string) bool {
	if runtime.GOOS == "windows" {
		path, root = strings.ToLower(path), strings.ToLower(root)
	}
	if path == root {
		return true
	}
	if !strings.HasSuffix(root, string(filepath.Separator)) {
		root += string(filepath.Separator)
	}
	return strings.HasPrefix(path, root)
}

// Allowed resolves path and checks it lies inside a root (after symlinks).
func (f *Files) Allowed(path string) (string, error) {
	if path == "" {
		return "", proto.Errorf(proto.ErrBadRequest, "empty path")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", proto.Errorf(proto.ErrBadRequest, "bad path")
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", proto.Errorf(proto.ErrNotFound, "path not found")
	}
	for _, r := range f.roots {
		rr, err := filepath.EvalSymlinks(r)
		if err != nil {
			rr = r
		}
		if samePrefix(real, rr) {
			return real, nil
		}
	}
	return "", proto.Errorf(proto.ErrForbidden, "path outside allowed folders")
}

// List returns the entries of a directory, folders first.
func (f *Files) List(path string) ([]proto.FsEntry, error) {
	real, err := f.Allowed(path)
	if err != nil {
		return nil, err
	}
	des, err := os.ReadDir(real)
	if err != nil {
		return nil, proto.Errorf(proto.ErrForbidden, "cannot read folder")
	}
	out := make([]proto.FsEntry, 0, len(des))
	for _, de := range des {
		name := de.Name()
		if strings.HasPrefix(name, ".") || strings.EqualFold(name, "desktop.ini") || strings.HasPrefix(name, "$") {
			continue
		}
		info, err := de.Info()
		if err != nil {
			continue
		}
		e := proto.FsEntry{Name: name, Path: filepath.Join(real, name), Dir: de.IsDir(), MTime: info.ModTime().Unix()}
		if !e.Dir {
			e.Size = info.Size()
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dir != out[j].Dir {
			return out[i].Dir
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	if len(out) > 2000 {
		out = out[:2000]
	}
	return out, nil
}

// Open opens a file inside the roots with the default handler.
func (f *Files) Open(shell Shell, path string) error {
	real, err := f.Allowed(path)
	if err != nil {
		return err
	}
	if err := shell.Open(real); err != nil {
		return fmt.Errorf("open: %w", err)
	}
	return nil
}
