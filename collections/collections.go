package collections

import "sync"

func Def[T comparable](val, def T) T {
	var empty T
	if val == empty {
		return def
	}
	return val
}

type SafeSlice[T any] struct {
	sync.Mutex
	s []T
}

func (s *SafeSlice[T]) Append(items ...T) {
	s.Lock()
	defer s.Unlock()
	s.s = append(s.s, items...)
}

func (s *SafeSlice[T]) Get() []T {
	s.Lock()
	defer s.Unlock()
	return s.s
}

type Empty struct{}
