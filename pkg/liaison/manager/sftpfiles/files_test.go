package sftpfiles

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"github.com/stretchr/testify/require"
)

// Use the actual SFTP wire protocol and server, not a fake filesystem adapter.
func fixture(t *testing.T, writable bool, options ...sftp.ServerOption) (*Files, string) {
	t.Helper()
	left, right := net.Pipe()
	server, err := sftp.NewServer(right, options...)
	require.NoError(t, err)
	finished := make(chan error, 1)
	go func() { finished <- server.Serve() }()
	client, err := sftp.NewClientPipe(left, left)
	require.NoError(t, err)
	f := &Files{client: client, transport: left, slot: make(chan struct{}, 1), writable: writable}
	t.Cleanup(func() {
		require.NoError(t, f.Close())
		require.NoError(t, right.Close())
		select {
		case err := <-finished:
			if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("server: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("SFTP server did not stop")
		}
	})
	return f, t.TempDir()
}

func TestFiles_RoundTripAndNoOverwrite(t *testing.T) {
	f, root := fixture(t, true)
	ctx := context.Background()
	p := filepath.Join(root, "中文 notes.txt")
	payload := "hello 世界\n"
	n, err := f.Upload(ctx, p, strings.NewReader(payload), 1024)
	require.NoError(t, err)
	require.EqualValues(t, len(payload), n)
	entries, err := f.List(ctx, root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "中文 notes.txt", entries[0].Name)
	preview, err := f.Preview(ctx, p)
	require.NoError(t, err)
	require.Equal(t, payload, preview)
	var dst bytes.Buffer
	n, err = f.Download(ctx, p, &dst)
	require.NoError(t, err)
	require.EqualValues(t, len(payload), n)
	require.Equal(t, payload, dst.String())
	_, err = f.Upload(ctx, p, strings.NewReader("replace"), 1024)
	require.ErrorIs(t, err, os.ErrExist)
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, payload, string(data))
	info, err := os.Stat(p)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0600), info.Mode().Perm())
}

func TestFiles_EmptyFileAndOversizedUploadCleanup(t *testing.T) {
	f, root := fixture(t, true)
	ctx := context.Background()
	_, err := f.Upload(ctx, filepath.Join(root, "empty"), strings.NewReader(""), 0)
	require.NoError(t, err)
	_, err = f.Upload(ctx, filepath.Join(root, "large"), strings.NewReader("12345"), 4)
	require.ErrorIs(t, err, ErrTooLarge)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "empty", entries[0].Name())
}

func TestFiles_PreviewRejectsBinaryOversizeDirectoryAndSymlink(t *testing.T) {
	f, root := fixture(t, false)
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		data []byte
		want error
	}{
		{"nul", []byte{'a', 0}, ErrBinary}, {"invalid-utf8", []byte{0xff}, ErrBinary}, {"large", bytes.Repeat([]byte{'x'}, PreviewLimit+1), ErrTooLarge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(root, tc.name)
			require.NoError(t, os.WriteFile(p, tc.data, 0600))
			_, err := f.Preview(ctx, p)
			require.ErrorIs(t, err, tc.want)
		})
	}
	_, err := f.Preview(ctx, root)
	require.ErrorIs(t, err, ErrNotRegular)
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(filepath.Join(root, "nul"), link))
	_, err = f.Preview(ctx, link)
	require.ErrorIs(t, err, ErrNotRegular)
	entries, err := f.List(ctx, root)
	require.NoError(t, err)
	found := false
	for _, e := range entries {
		if e.Name == "link" {
			found = e.Symlink
		}
	}
	require.True(t, found)
}

func TestFiles_ReadOnlyAndInvalidInput(t *testing.T) {
	f, root := fixture(t, false)
	ctx := context.Background()
	_, err := f.Upload(ctx, filepath.Join(root, "blocked"), strings.NewReader("x"), 1)
	require.ErrorIs(t, err, ErrReadOnly)
	for _, p := range []string{"", "a\x00b", "a\rb", "a\nb", strings.Repeat("x", 4097)} {
		_, err = f.List(ctx, p)
		require.ErrorIs(t, err, ErrInvalidPath)
	}
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, err = f.List(ctx, root)
	require.ErrorIs(t, err, context.Canceled)
}

func TestFiles_RemoteReadOnlyIsEnforced(t *testing.T) {
	f, root := fixture(t, true, sftp.ReadOnly())
	_, err := f.Upload(context.Background(), filepath.Join(root, "blocked"), strings.NewReader("x"), 1)
	require.Error(t, err)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}

type raceReader struct {
	path string
	done bool
}

func (r *raceReader) Read(p []byte) (int, error) {
	if r.done {
		return 0, io.EOF
	}
	r.done = true
	if err := os.WriteFile(r.path, []byte("other writer"), 0600); err != nil {
		return 0, err
	}
	return copy(p, "new payload"), nil
}
func TestFiles_ConcurrentDestinationCreationDoesNotOverwrite(t *testing.T) {
	f, root := fixture(t, true)
	p := filepath.Join(root, "race")
	_, err := f.Upload(context.Background(), p, &raceReader{path: p}, 100)
	require.Error(t, err)
	data, err := os.ReadFile(p)
	require.NoError(t, err)
	require.Equal(t, "other writer", string(data))
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestFiles_CancelAndBusy(t *testing.T) {
	f, _ := fixture(t, true)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done, err := f.begin(ctx)
	require.NoError(t, err)
	_, err = f.begin(context.Background())
	require.ErrorIs(t, err, ErrBusy)
	cancel()
	finished := make(chan error, 1)
	go func() { finished <- f.client.Wait() }()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cancel did not close transport")
	}
	done()
}
