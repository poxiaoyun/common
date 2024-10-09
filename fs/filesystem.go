package fs

import (
	"errors"
	"io"
	"io/fs"
	"syscall"
)

type (
	DirEntry = fs.DirEntry
	FileMode = fs.FileMode
	FileInfo = fs.FileInfo
)

const (
	ModeDir     FileMode = fs.ModeDir
	ModeSymlink FileMode = fs.ModeSymlink
	ModePerm    FileMode = 0o777 // Unix permission bits
)

// Flags to OpenFile wrapping those of the underlying system. Not all
// flags may be implemented on a given system.
const (
	// Exactly one of O_RDONLY, O_WRONLY, or O_RDWR must be specified.
	O_RDONLY int = syscall.O_RDONLY // open the file read-only.
	O_WRONLY int = syscall.O_WRONLY // open the file write-only.
	O_RDWR   int = syscall.O_RDWR   // open the file read-write.
	// The remaining values may be or'ed in to control behavior.
	O_APPEND int = syscall.O_APPEND // append data to the file when writing.
	O_CREATE int = syscall.O_CREAT  // create a new file if none exists.
	O_EXCL   int = syscall.O_EXCL   // used with O_CREATE, file must not exist.
	O_SYNC   int = syscall.O_SYNC   // open for synchronous I/O.
	O_TRUNC  int = syscall.O_TRUNC  // truncate regular writable file when opened.
)

var (
	ErrReadOnly     = errors.New("read-only filesystem")
	ErrNotSupported = errors.New("feature not supported")
)

type FileSystem interface {
	// ReadDir reads the named directory
	// and returns a list of directory entries sorted by filename.
	ReadDir(name string) ([]DirEntry, error)

	// Sub returns an FS corresponding to the subtree rooted at dir.
	Sub(dir string) (FileSystem, error)

	// OpenFile is the generalized open call; most users will use Open or Create
	// instead. It opens the named file with specified flag (O_RDONLY etc.) and
	// perm, (0666 etc.) if applicable. If successful, methods on the returned
	// File can be used for I/O.
	OpenFile(name string, flag int, perm FileMode) (File, error)

	// Stat returns a FileInfo describing the named file.
	Stat(name string) (FileInfo, error)

	// Rename renames (moves) oldpath to newpath. If newpath already exists and
	// is not a directory, Rename replaces it. OS-specific restrictions may
	// apply when oldpath and newpath are in different directories.
	Rename(oldpath, newpath string) error

	// Remove removes the named file or directory.
	Remove(name string) error

	// RemoveAll removes path and any children it contains.
	// It removes everything it can but returns the first error
	// it encounters. If the path does not exist, RemoveAll
	// returns nil (no error).
	// If there is an error, it will be of type [*fs.PathError].
	RemoveAll(path string) error

	// MkdirAll creates a directory named path, along with any necessary
	// parents, and returns nil, or else returns an error. The permission bits
	// perm are used for all directories that MkdirAll creates. If path is/
	// already a directory, MkdirAll does nothing and returns nil.
	MkdirAll(name string, perm FileMode) error

	// Lstat returns a FileInfo describing the named file. If the file is a
	// symbolic link, the returned FileInfo describes the symbolic link. Lstat
	// makes no attempt to follow the link.
	Lstat(name string) (FileInfo, error)

	// Symlink creates a symbolic-link from link to target. target may be an
	// absolute or relative path, and need not refer to an existing node.
	// Parent directories of link are created as necessary.
	Symlink(target, link string) error

	// Readlink returns the target path of link.
	Readlink(link string) (string, error)
}

// File represent a file, being a subset of the os.File
type File interface {
	io.Reader
	io.Writer
	io.Closer
	io.Seeker
}
