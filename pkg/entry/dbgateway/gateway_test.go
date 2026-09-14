package dbgateway

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/liaisonio/liaison/pkg/entry/firewall"
	"github.com/liaisonio/liaison/pkg/proto"
	"github.com/singchia/geminio"
	"github.com/stretchr/testify/require"
)

type testStream struct {
	geminio.Stream
	net.Conn
}

// Explicit methods resolve the two embedded interfaces' common methods.
func (s *testStream) Read(p []byte) (int, error)         { return s.Conn.Read(p) }
func (s *testStream) Write(p []byte) (int, error)        { return s.Conn.Write(p) }
func (s *testStream) Close() error                       { return s.Conn.Close() }
func (s *testStream) LocalAddr() net.Addr                { return s.Conn.LocalAddr() }
func (s *testStream) RemoteAddr() net.Addr               { return s.Conn.RemoteAddr() }
func (s *testStream) SetDeadline(t time.Time) error      { return s.Conn.SetDeadline(t) }
func (s *testStream) SetReadDeadline(t time.Time) error  { return s.Conn.SetReadDeadline(t) }
func (s *testStream) SetWriteDeadline(t time.Time) error { return s.Conn.SetWriteDeadline(t) }

type testFrontier struct {
	open  func(context.Context, uint64) (geminio.Stream, error)
	calls atomic.Int64
}

func (f *testFrontier) OpenStream(ctx context.Context, id uint64) (geminio.Stream, error) {
	f.calls.Add(1)
	return f.open(ctx, id)
}
func (f *testFrontier) Close() error { return nil }

func testProxy() *proto.Proxy {
	return &proto.Proxy{ID: 7, EdgeID: 42, ApplicationID: 12, Dst: "database.internal:5432", AccessProtocol: "postgresql"}
}
func connect(t *testing.T, p *proto.Proxy) net.Conn {
	t.Helper()
	c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p.ProxyPort), time.Second)
	require.NoError(t, err)
	t.Cleanup(func() { c.Close() })
	require.NoError(t, c.SetDeadline(time.Now().Add(3*time.Second)))
	return c
}

func TestGatewayRejectsSourceBeforeConnectorDial(t *testing.T) {
	f := &testFrontier{}
	fw := firewall.NewManager()
	require.NoError(t, fw.Allow(7, nil))
	g := New(f, fw, nil)
	t.Cleanup(g.Close)
	p := testProxy()
	require.NoError(t, g.CreateProxy(context.Background(), p))
	c := connect(t, p)
	_, err := c.Read(make([]byte, 1))
	require.Error(t, err)
	require.Zero(t, f.calls.Load())
}

func TestGatewayDeleteCancelsConnectorDial(t *testing.T) {
	started := make(chan struct{})
	f := &testFrontier{open: func(ctx context.Context, _ uint64) (geminio.Stream, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	g := New(f, firewall.NewManager(), nil)
	t.Cleanup(g.Close)
	p := testProxy()
	require.NoError(t, g.CreateProxy(context.Background(), p))
	c := connect(t, p)
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("dial not started")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, g.DeleteProxy(ctx, p.ID))
	_, err := c.Read(make([]byte, 1))
	require.Error(t, err)
	// The same port is available immediately after deletion completes.
	require.NoError(t, g.CreateProxy(context.Background(), p))
}

func TestGatewayRoutesRegisteredTargetAndClosesNegotiation(t *testing.T) {
	server, upstream := net.Pipe()
	defer server.Close()
	defer upstream.Close()
	f := &testFrontier{open: func(_ context.Context, id uint64) (geminio.Stream, error) {
		if id != 42 {
			return nil, fmt.Errorf("unexpected connector")
		}
		return &testStream{Conn: upstream}, nil
	}}
	g := New(f, firewall.NewManager(), nil)
	t.Cleanup(g.Close)
	p := testProxy()
	requestCtx, requestCancel := context.WithCancel(context.Background())
	require.NoError(t, g.CreateProxy(requestCtx, p))
	requestCancel() // Finishing the creation HTTP request must not stop access.
	c := connect(t, p)
	require.NoError(t, server.SetDeadline(time.Now().Add(3*time.Second)))
	header := make([]byte, 4)
	_, err := io.ReadFull(server, header)
	require.NoError(t, err)
	n := binary.BigEndian.Uint32(header)
	require.Less(t, n, uint32(4096))
	payload := make([]byte, n)
	_, err = io.ReadFull(server, payload)
	require.NoError(t, err)
	var dst proto.Dst
	require.NoError(t, json.Unmarshal(payload, &dst))
	require.Equal(t, proto.Dst{Addr: p.Dst, ApplicationID: p.ApplicationID, ProxyID: uint(p.ID)}, dst)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	require.NoError(t, g.DeleteProxy(ctx, p.ID))
	_, err = c.Read(header)
	require.Error(t, err)
	_, err = server.Read(header)
	require.Error(t, err)
}

func TestGatewayClosedAndInvalidCreation(t *testing.T) {
	g := New(&testFrontier{}, firewall.NewManager(), nil)
	require.Error(t, g.CreateProxy(context.Background(), nil))
	p := testProxy()
	p.AccessProtocol = "tcp"
	require.Error(t, g.CreateProxy(context.Background(), p))
	g.Close()
	g.Close()
	require.ErrorIs(t, g.CreateProxy(context.Background(), testProxy()), net.ErrClosed)
}

func TestGatewayPolicyChangeCancelsPendingSession(t *testing.T) {
	started := make(chan struct{})
	f := &testFrontier{open: func(ctx context.Context, _ uint64) (geminio.Stream, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	fw := firewall.NewManager()
	g := New(f, fw, nil)
	t.Cleanup(g.Close)
	p := testProxy()
	require.NoError(t, g.CreateProxy(context.Background(), p))
	c := connect(t, p)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dial not started")
	}
	require.NoError(t, fw.Allow(p.ID, nil))
	_, err := c.Read(make([]byte, 1))
	require.Error(t, err)
	if timeout, ok := err.(net.Error); ok {
		require.False(t, timeout.Timeout(), "policy must close the connection")
	}
}

func TestGatewayCapacityRejectsWithoutDialing(t *testing.T) {
	started := make(chan struct{})
	f := &testFrontier{open: func(ctx context.Context, _ uint64) (geminio.Stream, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	g := New(f, firewall.NewManager(), nil)
	g.slots = make(chan struct{}, 1)
	t.Cleanup(g.Close)
	p := testProxy()
	require.NoError(t, g.CreateProxy(context.Background(), p))
	connect(t, p)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("dial not started")
	}
	second := connect(t, p)
	_, err := second.Read(make([]byte, 1))
	require.Error(t, err)
	require.Equal(t, int64(1), f.calls.Load())
}
