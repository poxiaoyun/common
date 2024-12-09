package fs

import (
	"io"
	"io/fs"
	"os"
	"path"
)

func Open(fsys FileSystem, name string) (File, error) {
	return fsys.OpenFile(name, os.O_RDONLY, 0)
}

func Create(f FileSystem, name string) (File, error) {
	return f.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o666)
}

func IsSymbolicLink(d FileInfo) bool {
	return d.Mode().Type()&fs.ModeSymlink != 0
}

type WalkDirFunc func(path string, d DirEntry, err error) error

// WalkDir walks the file tree rooted at root, calling fn for each file or
// directory in the tree, including root.
//
// All errors that arise visiting files and directories are filtered by fn:
// see the [fs.WalkDirFunc] documentation for details.
//
// The files are walked in lexical order, which makes the output deterministic
// but requires WalkDir to read an entire directory into memory before proceeding
// to walk that directory.
//
// WalkDir does not follow symbolic links found in directories,
// but if root itself is a symbolic link, its target will be walked.
func WalkDir(fsys FileSystem, root string, fn WalkDirFunc) error {
	info, err := fsys.Stat(root)
	if err != nil {
		err = fn(root, nil, err)
	} else {
		err = walkDir(fsys, root, fs.FileInfoToDirEntry(info), fn)
	}
	if err == fs.SkipDir || err == fs.SkipAll {
		return nil
	}
	return err
}

// walkDir recursively descends path, calling walkDirFn.
func walkDir(fsys FileSystem, name string, d DirEntry, walkDirFn WalkDirFunc) error {
	if err := walkDirFn(name, d, nil); err != nil || !d.IsDir() {
		if err == fs.SkipDir && d.IsDir() {
			// Successfully skipped directory.
			err = nil
		}
		return err
	}

	dirs, err := fsys.ReadDir(name)
	if err != nil {
		// Second call, to report ReadDir error.
		err = walkDirFn(name, d, err)
		if err != nil {
			if err == fs.SkipDir && d.IsDir() {
				err = nil
			}
			return err
		}
	}

	for _, d1 := range dirs {
		name1 := path.Join(name, d1.Name())
		if err := walkDir(fsys, name1, d1, walkDirFn); err != nil {
			if err == fs.SkipDir {
				break
			}
			return err
		}
	}
	return nil
}

func Copy(fsys FileSystem, dst, src string) error {
	finfo, err := fsys.Stat(src)
	if err != nil {
		return err
	}
	fmode := finfo.Mode()
	if fmode.IsDir() {
		return CopyDir(fsys, dst, src)
	}
	return CopyFile(fsys, dst, src)
}

func CopyDir(fsys FileSystem, dst, src string) error {
	if err := fsys.MkdirAll(dst, 0o777); err != nil {
		return err
	}
	entries, err := fsys.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := path.Join(src, entry.Name())
		dstPath := path.Join(dst, entry.Name())
		if entry.IsDir() {
			if err := CopyDir(fsys, dstPath, srcPath); err != nil {
				return err
			}
		} else {
			if err := CopyFile(fsys, dstPath, srcPath); err != nil {
				return err
			}
		}
	}
	return nil
}

func CopyFile(fsys FileSystem, dst, src string) error {
	srcFile, err := Open(fsys, src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := Create(fsys, dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err = io.Copy(dstFile, srcFile); err != nil {
		defer fsys.Remove(dst)
		return err
	}
	return nil
}
