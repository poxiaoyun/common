package fs

type TempFileSystem interface {
	FileSystem
	TempDir() string
	MkdirTemp(dir, pattern string) (string, error)
	CreateTemp(dir, pattern string) (name string, err error)
}
