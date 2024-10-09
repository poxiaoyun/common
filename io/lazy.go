package io

import "bytes"

type LazyBytesReader struct {
	Init   func() ([]byte, error)
	reader *bytes.Reader
}

func (r *LazyBytesReader) lazyinit() (err error) {
	if r.reader == nil {
		content, err := r.Init()
		if err != nil {
			return err
		}
		r.reader = bytes.NewReader(content)
	}
	return nil
}

func (r *LazyBytesReader) Read(p []byte) (n int, err error) {
	if err := r.lazyinit(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func (r *LazyBytesReader) Seek(offset int64, whence int) (int64, error) {
	if err := r.lazyinit(); err != nil {
		return 0, err
	}
	return r.reader.Seek(offset, whence)
}
