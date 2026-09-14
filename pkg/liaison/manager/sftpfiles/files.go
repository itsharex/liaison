// Package sftpfiles implements remote file operations, never local shell commands.
// Authentication, resource authorization and auditing belong to the caller.
package sftpfiles

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

var (
	ErrInvalidPath  = errors.New("invalid remote path")
	ErrNotRegular   = errors.New("not a regular file")
	ErrTooLarge     = errors.New("file exceeds size limit")
	ErrBinary       = errors.New("text preview requires UTF-8 text")
	ErrReadOnly     = errors.New("file session is read-only")
	ErrBusy         = errors.New("file session is busy")
	ErrAtomicUpload = errors.New("server does not support atomic no-overwrite uploads")
)

const PreviewLimit = 256 * 1024

const TransferLimit = 64 * 1024 * 1024

func (f *Files) Home(ctx context.Context) (string, error) {
	done, err := f.begin(ctx)
	if err != nil {
		return "", err
	}
	defer done()
	return f.client.RealPath(".")
}

func (f *Files) CanUpload() bool {
	_, ok := f.client.HasExtension("hardlink@openssh.com")
	return f.writable && ok
}

// Files owns a dedicated SSH connection. Canceling an operation closes this
// connection, not the user's terminal. A canceled session must be reopened.
type Files struct {
	client    *sftp.Client
	transport io.Closer
	once      sync.Once
	closeErr  error
	slot      chan struct{}
	writable  bool
}

// New starts SFTP on an already authenticated, host-key-verified SSH connection.
// Ownership of conn transfers to Files, including on initialization failure.
func New(ctx context.Context, conn *ssh.Client, writable bool) (*Files, error) {
	if conn == nil {
		return nil, errors.New("missing SSH connection")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() /* unblock subsystem negotiation */ })
	client, err := sftp.NewClient(conn, sftp.UseConcurrentReads(false), sftp.UseConcurrentWrites(false))
	stopped := stop()
	if err != nil || !stopped || ctx.Err() != nil {
		_ = conn.Close() // failed initialization owns and releases the transport
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("start SFTP: %w", err)
	}
	return &Files{client: client, transport: conn, slot: make(chan struct{}, 1), writable: writable}, nil
}

func (f *Files) Close() error {
	f.once.Do(func() { f.closeErr = f.transport.Close() })
	return f.closeErr
}

func (f *Files) begin(ctx context.Context) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	select {
	case f.slot <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	stop := context.AfterFunc(ctx, func() { _ = f.Close() /* unblock remote IO on cancellation */ })
	return func() { stop(); cancel(); <-f.slot }, nil
}

func validPath(p string) bool {
	return p != "" && len(p) <= 4096 && utf8.ValidString(p) && !strings.ContainsAny(p, "\x00\r\n")
}

type Entry struct {
	Name       string    `json:"name"`
	Size       int64     `json:"size"`
	Mode       string    `json:"mode"`
	Directory  bool      `json:"directory"`
	Symlink    bool      `json:"symlink"`
	ModifiedAt time.Time `json:"modified_at"`
}

func (f *Files) List(ctx context.Context, p string) ([]Entry, error) {
	if !validPath(p) {
		return nil, ErrInvalidPath
	}
	done, err := f.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer done()
	infos, err := f.client.ReadDirContext(ctx, p)
	if err != nil {
		return nil, err
	}
	// The underlying library materializes directory entries. Do not describe this
	// response limit as a server-side memory bound; pagination is a follow-up.
	if len(infos) > 10000 {
		return nil, ErrTooLarge
	}
	entries := make([]Entry, 0, len(infos))
	for _, i := range infos {
		entries = append(entries, Entry{i.Name(), i.Size(), i.Mode().String(), i.IsDir(), i.Mode()&os.ModeSymlink != 0, i.ModTime()})
	}
	return entries, nil
}

func (f *Files) regular(p string) (*sftp.File, error) {
	info, err := f.client.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, ErrNotRegular
	}
	file, err := f.client.Open(p)
	if err != nil {
		return nil, err
	}
	// Recheck the opened handle in case the remote entry changed after Lstat.
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close() // reject and release an unusable handle
		if err != nil {
			return nil, err
		}
		return nil, ErrNotRegular
	}
	return file, nil
}

func (f *Files) Preview(ctx context.Context, p string) (string, error) {
	if !validPath(p) {
		return "", ErrInvalidPath
	}
	done, err := f.begin(ctx)
	if err != nil {
		return "", err
	}
	defer done()
	file, err := f.regular(p)
	if err != nil {
		return "", err
	}
	data, readErr := io.ReadAll(io.LimitReader(file, PreviewLimit+1))
	err = errors.Join(readErr, file.Close())
	if err != nil {
		return "", err
	}
	if len(data) > PreviewLimit {
		return "", ErrTooLarge
	}
	if !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return "", ErrBinary
	}
	return string(data), ctx.Err()
}

// Download returns the actual copied byte count for auditing. The caller must
// ensure writes to dst unblock when ctx ends (e.g. an HTTP write deadline).
func (f *Files) Download(ctx context.Context, p string, dst io.Writer) (int64, error) {
	if !validPath(p) || dst == nil {
		return 0, ErrInvalidPath
	}
	done, err := f.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer done()
	file, err := f.regular(p)
	if err != nil {
		return 0, err
	}
	info, err := file.Stat()
	if err != nil || info.Size() > TransferLimit {
		_ = file.Close() // release rejected transfer handle
		if err != nil {
			return 0, err
		}
		return 0, ErrTooLarge
	}
	// Hide WriterTo to keep transfer buffers bounded and avoid library fan-out.
	n, err := io.CopyBuffer(dst, io.LimitReader(file, TransferLimit+1), make([]byte, 32*1024))
	if n > TransferLimit {
		err = errors.Join(err, ErrTooLarge)
	}
	return n, errors.Join(err, file.Close(), ctx.Err())
}

// Upload requires the OpenSSH hardlink extension: publishing a complete temporary
// file with Link cannot overwrite an existing destination, even under a race.
// The caller must unblock src on cancellation. A cleanup error is returned with
// the temporary remote path so it can be audited/retried after reconnecting.
func (f *Files) Upload(ctx context.Context, p string, src io.Reader, limit int64) (n int64, err error) {
	if !f.writable {
		return 0, ErrReadOnly
	}
	if !validPath(p) || src == nil || limit < 0 || limit > 1<<40 {
		return 0, ErrInvalidPath
	}
	if _, ok := f.client.HasExtension("hardlink@openssh.com"); !ok {
		return 0, ErrAtomicUpload
	}
	done, err := f.begin(ctx)
	if err != nil {
		return 0, err
	}
	defer done()
	if _, statErr := f.client.Lstat(p); statErr == nil {
		return 0, os.ErrExist
	} else if !os.IsNotExist(statErr) {
		return 0, statErr
	}
	var nonce [16]byte
	if _, err = rand.Read(nonce[:]); err != nil {
		return 0, err
	}
	temporary := path.Join(path.Dir(p), ".liaison-upload-"+hex.EncodeToString(nonce[:]))
	file, err := f.client.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cleanupErr := f.client.Remove(temporary); cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("cleanup remote temporary file %q: %w", temporary, cleanupErr))
		}
	}()
	if err = file.Chmod(0600); err != nil {
		return 0, errors.Join(err, file.Close())
	}
	n, err = io.CopyBuffer(struct{ io.Writer }{file}, io.LimitReader(src, limit+1), make([]byte, 32*1024))
	err = errors.Join(err, file.Close(), ctx.Err())
	if err != nil {
		return n, err
	}
	if n > limit {
		return n, ErrTooLarge
	}
	if err = f.client.Link(temporary, p); err != nil {
		return n, err
	}
	return n, nil
}
