package fs

import (
	"os"
)

var _ FileSystem = &OSFileSystem{}

type OSFileSystem struct{}

// Lstat implements FileSystem.
func (o *OSFileSystem) Lstat(filename string) (FileInfo, error) {
	return os.Lstat(filename)
}

// MkdirAll implements FileSystem.
func (o *OSFileSystem) MkdirAll(filename string, perm FileMode) error {
	return os.MkdirAll(filename, os.FileMode(perm))
}

// OpenFile implements FileSystem.
func (o *OSFileSystem) OpenFile(filename string, flag int, perm FileMode) (File, error) {
	return os.OpenFile(filename, flag, os.FileMode(perm))
}

// ReadDir implements FileSystem.
func (o *OSFileSystem) ReadDir(name string) ([]DirEntry, error) {
	dirs, err := os.ReadDir(name)
	if err != nil {
		return nil, err
	}
	entries := make([]DirEntry, 0, len(dirs))
	for _, dir := range dirs {
		entries = append(entries, dir)
	}
	return entries, nil
}

// Readlink implements FileSystem.
func (o *OSFileSystem) Readlink(link string) (string, error) {
	return os.Readlink(link)
}

// Remove implements FileSystem.
func (o *OSFileSystem) Remove(filename string) error {
	return os.Remove(filename)
}

// RemoveAll implements FileSystem.
func (o *OSFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Rename implements FileSystem.
func (o *OSFileSystem) Rename(oldpath string, newpath string) error {
	return os.Rename(oldpath, newpath)
}

// Stat implements FileSystem.
func (o *OSFileSystem) Stat(filename string) (FileInfo, error) {
	return os.Stat(filename)
}

// Sub implements FileSystem.
func (o *OSFileSystem) Sub(dir string) (FileSystem, error) {
	return SubFS{Fsys: o, Dir: dir}, nil
}

// Symlink implements FileSystem.
func (o *OSFileSystem) Symlink(target string, link string) error {
	return os.Symlink(target, link)
}
