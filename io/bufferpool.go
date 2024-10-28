package io

import (
	"bytes"
	"sync"
)

type BufferPool struct {
	p sync.Pool
}

func NewBufferPool(size int) *BufferPool {
	return &BufferPool{
		p: sync.Pool{
			New: func() any {
				var b []byte
				if size > 0 {
					b = make([]byte, size)
				}
				return bytes.NewBuffer(b)
			},
		},
	}
}

func (p *BufferPool) Get() *bytes.Buffer {
	return p.p.Get().(*bytes.Buffer)
}

func (p *BufferPool) Put(b *bytes.Buffer) {
	b.Reset()
	p.p.Put(b)
}
