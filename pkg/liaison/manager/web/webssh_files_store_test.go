package web

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestFileSessionIsolationAndCancellation(t *testing.T) {
	s := newFileSessions()
	defer s.close()
	v := fileSession{id: "fixture", userID: 1, auth: fileAuth("first-login"), expires: time.Now().Add(time.Minute)}
	if err := s.add(v); err != nil {
		t.Fatal(err)
	}
	for _, other := range []struct {
		user uint
		auth [32]byte
	}{{2, v.auth}, {1, fileAuth("second-login")}} {
		if _, _, err := s.acquire(v.id, other.user, other.auth, func() {}); !errors.Is(err, errFileSessionMissing) {
			t.Fatal("session exposed", err)
		}
		if s.remove(v.id, other.user, other.auth) {
			t.Fatal("foreign session removed")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, release, err := s.acquire(v.id, v.userID, v.auth, cancel)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = s.acquire(v.id, v.userID, v.auth, cancel); !errors.Is(err, errFileSessionBusy) {
		t.Fatal(err)
	}
	release()
	_, release, err = s.acquire(v.id, v.userID, v.auth, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if !s.remove(v.id, v.userID, v.auth) {
		t.Fatal("remove failed")
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("transfer not canceled")
	}
}

func TestFileSessionExpiryAndClose(t *testing.T) {
	s := newFileSessions()
	defer s.close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	v := fileSession{id: "expires", userID: 1, auth: fileAuth("login"), expires: time.Now().Add(50 * time.Millisecond)}
	if err := s.add(v); err != nil {
		t.Fatal(err)
	}
	_, release, err := s.acquire(v.id, 1, v.auth, cancel)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("expiration did not cancel")
	}
	if _, _, err = s.acquire(v.id, 1, v.auth, cancel); !errors.Is(err, errFileSessionMissing) {
		t.Fatal(err)
	}
	v.id = "close"
	v.expires = time.Now().Add(time.Minute)
	if err = s.add(v); err != nil {
		t.Fatal(err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	_, _, err = s.acquire(v.id, 1, v.auth, cancel2)
	if err != nil {
		t.Fatal(err)
	}
	s.close()
	select {
	case <-ctx2.Done():
	default:
		t.Fatal("close did not cancel")
	}
}
