package cacheaccess

import (
	"bufio"
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type noDeadlineConn struct{ net.Conn }

func (noDeadlineConn) SetDeadline(time.Time) error { panic("connector deadline API must not be used") }

func TestMemcachedConnectorDeadlineUsesCancellation(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := Execute(ctx, noDeadlineConn{client}, Command{Operation: "stats"}, false)
		done <- err
	}()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("deadline did not close blocked connector IO")
	}
}

func TestMemcachedValidation(t *testing.T) {
	for _, cmd := range []Command{
		{Operation: "flush_all"}, {Operation: "get", Key: "a\r\nflush_all"},
		{Operation: "get", Key: strings.Repeat("k", 251)}, {Operation: "get", Key: "a b"},
		{Operation: "set", Key: "k", TTLSeconds: 2592001}, {Operation: "set", Key: "k", Value: make([]byte, maxValueBytes+1)},
		{Operation: "stats", Key: "items"}, {Operation: "get", Key: "k", Value: []byte("v")},
	} {
		require.ErrorIs(t, cmd.Validate(), ErrInvalid)
	}
}

func TestMemcachedResponses(t *testing.T) {
	for _, tc := range []struct {
		name              string
		cmd               Command
		request, response string
		valid             bool
	}{
		{"get", Command{Operation: "get", Key: "k"}, "get k\r\n", "VALUE k 7 3\r\na\x00b\r\nEND\r\n", true},
		{"missing", Command{Operation: "get", Key: "k"}, "get k\r\n", "END\r\n", true},
		{"stats", Command{Operation: "stats"}, "stats\r\n", "STAT curr_items 2\r\nEND\r\n", true},
		{"set", Command{Operation: "set", Key: "k", Value: []byte("a\nb"), TTLSeconds: 60}, "set k 0 60 3\r\na\nb\r\n", "STORED\r\n", true},
		{"delete", Command{Operation: "delete", Key: "k"}, "delete k\r\n", "DELETED\r\n", true},
		{"wrong key", Command{Operation: "get", Key: "k"}, "get k\r\n", "VALUE other 0 1\r\nx\r\nEND\r\n", false},
		{"oversize", Command{Operation: "get", Key: "k"}, "get k\r\n", "VALUE k 0 99999999\r\n", false},
		{"truncated", Command{Operation: "get", Key: "k"}, "get k\r\n", "VALUE k 0 5\r\nx", false},
		{"malformed", Command{Operation: "stats"}, "stats\r\n", "STAT x 1\nEND\r\n", false},
		{"long line", Command{Operation: "stats"}, "stats\r\n", strings.Repeat("x", 8192), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer server.Close()
			done := make(chan string, 1)
			go func() {
				defer server.Close()
				input := make([]byte, len(tc.request))
				if _, err := io.ReadFull(server, input); err != nil {
					done <- "read failed"
					return
				}
				done <- string(input)
				// Client may reject malformed responses early and close the pipe.
				server.Write([]byte(tc.response))
			}()
			result, err := Execute(context.Background(), client, tc.cmd, true)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, tc.request, <-done)
			if tc.name == "get" {
				require.True(t, result.Found)
				require.Equal(t, []byte("a\x00b"), result.Value)
				require.Equal(t, uint32(7), result.Flags)
			}
			if tc.name == "missing" {
				require.False(t, result.Found)
			}
			if tc.name == "stats" {
				require.Equal(t, "2", result.Stats["curr_items"])
			}
		})
	}
}

func TestMemcachedWriteDeniedBeforeIO(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	_, err := Execute(context.Background(), client, Command{Operation: "delete", Key: "k"}, false)
	require.ErrorIs(t, err, ErrWriteDenied)
	_, err = server.Read(make([]byte, 1))
	require.ErrorIs(t, err, io.EOF)
}

func TestMemcachedCancellation(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := Execute(ctx, client, Command{Operation: "stats"}, false); done <- err }()
	_, err := bufio.NewReader(server).ReadString('\n')
	require.NoError(t, err)
	cancel()
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(time.Second):
		t.Fatal("cancellation did not close stream")
	}
}
