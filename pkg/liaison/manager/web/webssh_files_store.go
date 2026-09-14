package web

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"time"
)

var errFileSessionMissing = errors.New("file session not found")
var errFileSessionBusy = errors.New("file session busy")

type fileSession struct {
	id                         string
	userID, proxyID            uint
	auth                       [32]byte
	username, encrypted, nonce string
	expires                    time.Time
	cancel                     context.CancelFunc
	timer                      *time.Timer
}
type fileSessions struct {
	mu    sync.Mutex
	items map[string]*fileSession
}

func newFileSessions() *fileSessions { return &fileSessions{items: map[string]*fileSession{}} }
func (s *fileSessions) add(v fileSession) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	for _, x := range s.items {
		if x.userID == v.userID {
			count++
		}
	}
	if count >= 4 || len(s.items) >= 64 {
		return errFileSessionBusy
	}
	v.timer = time.AfterFunc(time.Until(v.expires), func() { s.remove(v.id, v.userID, v.auth) })
	s.items[v.id] = &v
	return nil
}
func (s *fileSessions) acquire(id string, user uint, auth [32]byte, cancel context.CancelFunc) (fileSession, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v := s.items[id]
	if v == nil || v.userID != user || v.auth != auth || time.Now().After(v.expires) {
		return fileSession{}, nil, errFileSessionMissing
	}
	if v.cancel != nil {
		return fileSession{}, nil, errFileSessionBusy
	}
	v.cancel = cancel
	copy := *v
	return copy, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if current := s.items[id]; current == v {
			current.cancel = nil
		}
	}, nil
}
func (s *fileSessions) remove(id string, user uint, auth [32]byte) bool {
	s.mu.Lock()
	v := s.items[id]
	if v == nil || v.userID != user || v.auth != auth {
		s.mu.Unlock()
		return false
	}
	delete(s.items, id)
	v.timer.Stop()
	cancel := v.cancel
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return true
}
func (s *fileSessions) close() {
	s.mu.Lock()
	var cancels []context.CancelFunc
	for id, v := range s.items {
		v.timer.Stop()
		if v.cancel != nil {
			cancels = append(cancels, v.cancel)
		}
		delete(s.items, id)
	}
	s.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}
func fileAuth(token string) [32]byte { return sha256.Sum256([]byte(token)) }
