package progress

import (
	"io"

	"github.com/google/uuid"
)

func (b *Bar) WrapReader(rc io.ReadCloser, name string, total int64, initStatus string) io.ReadCloser {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.Total = total
	b.Name = name
	b.Status = initStatus
	b.Done = false // reset done
	defer b.Notify()

	if b.Fragments == nil {
		b.Fragments = make(map[string]*BarFragment)
	}
	uid := uuid.New().String()
	thisfragment := &BarFragment{
		uid: uid,
	}
	b.Fragments[uid] = thisfragment

	barr := barr{ReadCloser: rc, fragment: thisfragment, b: b}
	if seeker, ok := rc.(io.ReadSeekCloser); ok {
		return &seekbarr{seeker: seeker, barr: barr}
	}
	return &barr
}

var _ io.ReadSeekCloser = &seekbarr{}

type seekbarr struct {
	seeker io.Seeker
	barr
}

func (r *seekbarr) Seek(offset int64, whence int) (int64, error) {
	r.fragment.Size = 0 // reset processed
	n, err := r.seeker.Seek(offset, whence)
	if err != nil {
		r.b.Status = "failed"
		r.b.Done = true
	}
	switch whence {
	case io.SeekStart:
		r.fragment.Offset = n
	case io.SeekCurrent:
		r.fragment.Offset += n
	case io.SeekEnd:
		r.fragment.Offset = r.b.Total - n
	}
	r.b.Notify()
	return n, err
}

var _ io.ReadCloser = &barr{}

type barr struct {
	io.ReadCloser
	fragment *BarFragment
	haserr   bool
	b        *Bar
}

func (r *barr) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if err != nil && err != io.EOF {
		r.b.Status = "failed"
		r.b.Done = true
		r.haserr = true
	}
	r.fragment.Size += int64(n)
	r.b.Notify()
	return n, err
}

func (r *barr) Close() error {
	if r.haserr {
		r.b.mu.Lock()
		defer r.b.mu.Unlock()
		delete(r.b.Fragments, r.fragment.uid)
	}

	return r.ReadCloser.Close()
}

func (b *Bar) WrapWriter(wc io.WriteCloser, name string, total int64, initStatus string) io.WriteCloser {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.Name = name
	b.Total = total
	b.Status = initStatus
	b.Notify()

	if b.Fragments == nil {
		b.Fragments = make(map[string]*BarFragment)
	}
	uid := uuid.New().String()
	thisfragment := &BarFragment{
		uid: uid,
	}
	b.Fragments[uid] = thisfragment

	w := &barw{fragment: thisfragment, wc: wc, b: b}
	if _, ok := wc.(io.WriterAt); ok {
		return barwa{barw: w}
	}
	return w
}

type barw struct {
	fragment *BarFragment
	wc       io.WriteCloser
	b        *Bar
	haserr   bool
}

func (r *barw) Write(p []byte) (int, error) {
	n, err := r.wc.Write(p)
	if err != nil && err != io.EOF {
		r.b.Done = true
		r.b.Status = "failed"
		r.haserr = true
	}
	r.fragment.Size += int64(n)
	r.b.Notify()
	return n, err
}

func (r *barw) Close() error {
	if r.haserr {
		r.b.mu.Lock()
		defer r.b.mu.Unlock()
		delete(r.b.Fragments, r.fragment.uid)
	}
	return r.wc.Close()
}

type barwa struct {
	*barw
}

func (r barwa) WriteAt(p []byte, off int64) (int, error) {
	wat, ok := r.wc.(io.WriterAt)
	if !ok {
		return 0, io.ErrUnexpectedEOF
	}
	n, err := wat.WriteAt(p, off)
	if err != nil {
		r.b.Done = true
		r.b.Status = "failed"
		r.b.Notify()
		return n, err
	}
	r.fragment.Size += int64(n)
	r.b.Notify()
	return n, nil
}
