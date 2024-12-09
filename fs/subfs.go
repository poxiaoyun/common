package fs

import (
	"io/fs"
	"path"
	"strings"
)

var _ FileSystem = SubFS{}

type SubFS struct {
	Fsys FileSystem
	Dir  string
}

// Lstat implements FileSystem.
func (f SubFS) Lstat(filename string) (FileInfo, error) {
	full, err := f.fullName("lstat", filename)
	if err != nil {
		return nil, err
	}
	return f.Fsys.Lstat(full)
}

// MkdirAll implements FileSystem.
func (f SubFS) MkdirAll(filename string, perm FileMode) error {
	full, err := f.fullName("mkdir", filename)
	if err != nil {
		return err
	}
	return f.Fsys.MkdirAll(full, perm)
}

// OpenFile implements FileSystem.
func (f SubFS) OpenFile(filename string, flag int, perm FileMode) (File, error) {
	full, err := f.fullName("open", filename)
	if err != nil {
		return nil, err
	}
	return f.Fsys.OpenFile(full, flag, perm)
}

// Readlink implements FileSystem.
func (f SubFS) Readlink(link string) (string, error) {
	full, err := f.fullName("readlink", link)
	if err != nil {
		return "", err
	}
	return f.Fsys.Readlink(full)
}

// Remove implements FileSystem.
func (f SubFS) Remove(filename string) error {
	full, err := f.fullName("remove", filename)
	if err != nil {
		return err
	}
	return f.Fsys.Remove(full)
}

// RemoveAll implements FileSystem.
func (f SubFS) RemoveAll(path string) error {
	full, err := f.fullName("removeAll", path)
	if err != nil {
		return err
	}
	return f.Fsys.RemoveAll(full)
}

// Rename implements FileSystem.
func (f SubFS) Rename(oldpath string, newpath string) error {
	oldfull, err := f.fullName("rename", oldpath)
	if err != nil {
		return err
	}
	newfull, err := f.fullName("rename", newpath)
	if err != nil {
		return err
	}
	return f.Fsys.Rename(oldfull, newfull)
}

// Stat implements FileSystem.
func (f SubFS) Stat(filename string) (FileInfo, error) {
	full, err := f.fullName("stat", filename)
	if err != nil {
		return nil, err
	}
	return f.Fsys.Stat(full)
}

// Symlink implements FileSystem.
func (f SubFS) Symlink(target string, link string) error {
	full, err := f.fullName("symlink", link)
	if err != nil {
		return err
	}
	return f.Fsys.Symlink(target, full)
}

func (f SubFS) ReadDir(name string) ([]DirEntry, error) {
	full, err := f.fullName("read", name)
	if err != nil {
		return nil, err
	}
	dir, err := f.Fsys.ReadDir(full)
	return dir, f.fixErr(err)
}

func (f SubFS) Sub(dir string) (FileSystem, error) {
	if dir == "." {
		return f, nil
	}
	full, err := f.fullName("sub", dir)
	if err != nil {
		return nil, err
	}
	return &SubFS{f.Fsys, full}, nil
}

// fullName maps name to the fully-qualified name dir/name.
func (f SubFS) fullName(op string, name string) (string, error) {
	if name == "" || name == "." || name == "/" {
		return f.Dir, nil
	}
	if !fs.ValidPath(strings.TrimPrefix(name, "/")) {
		return "", &fs.PathError{Op: op, Path: name, Err: fs.ErrInvalid}
	}
	return path.Join(f.Dir, name), nil
}

// shorten maps name, which should start with f.dir, back to the suffix after f.dir.
func (f SubFS) shorten(name string) (rel string, ok bool) {
	if name == f.Dir {
		return ".", true
	}
	if len(name) >= len(f.Dir)+2 && name[len(f.Dir)] == '/' && name[:len(f.Dir)] == f.Dir {
		return name[len(f.Dir)+1:], true
	}
	return "", false
}

// fixErr shortens any reported names in PathErrors by stripping f.dir.
func (f SubFS) fixErr(err error) error {
	if e, ok := err.(*fs.PathError); ok {
		if short, ok := f.shorten(e.Path); ok {
			e.Path = short
		}
	}
	return err
}
