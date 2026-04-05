package rpc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

type Sessions[T any] struct {
	mu       sync.RWMutex
	items    map[string]T
	newValue func() T
}

func NewSessions[T any](newValue func() T) *Sessions[T] {
	if newValue == nil {
		panic("rpc: session factory is required")
	}
	return &Sessions[T]{
		items:    make(map[string]T),
		newValue: newValue,
	}
}

func (s *Sessions[T]) Create() (string, T, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for range 8 {
		id, err := newSessionID()
		if err != nil {
			var zero T
			return "", zero, err
		}
		if _, exists := s.items[id]; exists {
			continue
		}
		value := s.newValue()
		s.items[id] = value
		return id, value, nil
	}
	var zero T
	return "", zero, fmt.Errorf("generate unique session id")
}

func (s *Sessions[T]) Get(id string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	value, ok := s.items[id]
	return value, ok
}

func (s *Sessions[T]) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	return true
}

func (s *Sessions[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

func newSessionID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("read random session id: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}
