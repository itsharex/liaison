// Package dbgateway owns native database listeners. Database authentication and
// TLS terminate at the target database, not at Liaison.
package dbgateway

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/jumboframes/armorigo/log"
	"github.com/liaisonio/liaison/pkg/entry/frontierbound"
	relay "github.com/liaisonio/liaison/pkg/liaison/manager/dbgateway"
	"github.com/liaisonio/liaison/pkg/proto"
	"github.com/liaisonio/liaison/pkg/trafficconn"
)

type sourcePolicy interface{ CheckAddr(int, net.Addr) bool }
type recorder interface {
	RecordTraffic(uint, uint, int64, int64)
}

type Gateway struct {
	mu        sync.Mutex
	listeners map[int]*listener
	closed    bool
	frontier  frontierbound.FrontierBound
	policy    sourcePolicy
	traffic   recorder
	// Bound total native database sessions, including connector negotiation.
	slots chan struct{}
}

type listener struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func New(frontier frontierbound.FrontierBound, policy sourcePolicy, traffic recorder) *Gateway {
	return &Gateway{listeners: make(map[int]*listener), frontier: frontier, policy: policy, traffic: traffic, slots: make(chan struct{}, 1024)}
}

func (g *Gateway) CreateProxy(ctx context.Context, p *proto.Proxy) error {
	if p == nil || p.ID <= 0 || p.EdgeID == 0 || p.ApplicationID == 0 || p.Dst == "" || (p.AccessProtocol != "mysql" && p.AccessProtocol != "postgresql") {
		return errors.New("invalid native database access")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Listener lifetime is controlled by DeleteProxy/Close, not the HTTP request.
	lifetime, cancel := context.WithCancel(context.WithoutCancel(ctx))
	state := &listener{cancel: cancel, done: make(chan struct{})}
	g.mu.Lock()
	if g.closed {
		g.mu.Unlock()
		cancel()
		return net.ErrClosed
	}
	if _, exists := g.listeners[p.ID]; exists {
		g.mu.Unlock()
		cancel()
		return errors.New("database listener already exists")
	}
	g.listeners[p.ID] = state
	g.mu.Unlock()

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", fmt.Sprintf(":%d", p.ProxyPort))
	if err != nil {
		cancel()
		g.mu.Lock()
		delete(g.listeners, p.ID)
		g.mu.Unlock()
		close(state.done)
		return fmt.Errorf("listen for database access: %w", err)
	}
	p.ProxyPort = ln.Addr().(*net.TCPAddr).Port
	config := *p // Immutable routing; never use client-provided target metadata.
	go g.serve(lifetime, state, ln, config)
	return nil
}

func (g *Gateway) serve(ctx context.Context, state *listener, ln net.Listener, p proto.Proxy) {
	var sessions sync.WaitGroup
	stop := context.AfterFunc(ctx, func() { ln.Close() }) // Best-effort socket cleanup.
	defer close(state.done)
	defer sessions.Wait()
	defer stop()
	defer ln.Close()
	defer state.cancel()
	defer func() {
		if recover() != nil {
			log.Errorf("database listener failed: proxy_id=%d", p.ID)
		}
	}()
	for {
		client, err := ln.Accept()
		if err != nil {
			return
		}
		select {
		case g.slots <- struct{}{}:
			sessions.Add(1)
			go func() {
				defer sessions.Done()
				defer func() { <-g.slots }()
				defer client.Close()
				defer func() {
					if recover() != nil {
						log.Errorf("database session failed: proxy_id=%d", p.ID)
					}
				}()
				g.handle(ctx, client, p)
			}()
		default:
			client.Close() // Capacity exhausted: fail closed without opening a tunnel.
		}
	}
}

func (g *Gateway) handle(parent context.Context, client net.Conn, p proto.Proxy) {
	if parent.Err() != nil || g.policy == nil || !g.policy.CheckAddr(p.ID, client.RemoteAddr()) {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 24*time.Hour)
	defer cancel()
	policyDone := make(chan struct{})
	go func() {
		defer close(policyDone)
		defer func() {
			if recover() != nil {
				cancel()
			}
		}()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !g.policy.CheckAddr(p.ID, client.RemoteAddr()) {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-policyDone }()
	stopClient := context.AfterFunc(ctx, func() { client.Close() })
	defer stopClient()
	dialCtx, stopDial := context.WithTimeout(ctx, 10*time.Second)
	upstream, err := g.frontier.OpenStream(dialCtx, p.EdgeID)
	stopDial()
	if err != nil {
		return
	}
	defer upstream.Close()
	stopUpstream := context.AfterFunc(ctx, func() { upstream.Close() })
	defer stopUpstream()
	if err := upstream.SetWriteDeadline(time.Now().Add(10 * time.Second)); err != nil {
		return
	}
	data, err := json.Marshal(proto.Dst{Addr: p.Dst, ApplicationID: p.ApplicationID, ProxyID: uint(p.ID)})
	if err != nil {
		return
	}
	header := make([]byte, 4)
	binary.BigEndian.PutUint32(header, uint32(len(data)))
	if err := writeAll(upstream, append(header, data...)); err != nil {
		return
	}
	if err := upstream.SetWriteDeadline(time.Time{}); err != nil {
		return
	}
	var target net.Conn = upstream
	if g.traffic != nil {
		target = trafficconn.TargetConn(target, g.traffic, uint(p.ID), p.ApplicationID)
	}
	// Idle deadlines are refreshed per IO; fixed negotiation deadlines still win.
	err = relay.Relay(ctx, &idleConn{Conn: client}, &idleConn{Conn: target}, p.AccessProtocol)
	if err != nil && ctx.Err() == nil && !errors.Is(err, io.EOF) && !errors.Is(err, net.ErrClosed) {
		// Do not log raw driver errors, credentials, or packet contents.
		log.Debugf("database connection ended: proxy_id=%d protocol=%s", p.ID, p.AccessProtocol)
	}
}

func (g *Gateway) DeleteProxy(ctx context.Context, id int) error {
	g.mu.Lock()
	state := g.listeners[id]
	g.mu.Unlock()
	if state == nil {
		return nil
	}
	state.cancel()
	select {
	case <-state.done:
		g.mu.Lock()
		if g.listeners[id] == state {
			delete(g.listeners, id)
		}
		g.mu.Unlock()
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *Gateway) Close() {
	g.mu.Lock()
	g.closed = true
	states := make([]*listener, 0, len(g.listeners))
	for _, state := range g.listeners {
		states = append(states, state)
	}
	g.mu.Unlock()
	for _, state := range states {
		state.cancel()
	}
	for _, state := range states {
		<-state.done
	}
}

func writeAll(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

type idleConn struct {
	net.Conn
	mu       sync.Mutex
	deadline time.Time
}

func (c *idleConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.deadline = t
	c.mu.Unlock()
	return c.Conn.SetDeadline(t)
}

func (c *idleConn) nextDeadline() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := time.Now().Add(15 * time.Minute)
	if !c.deadline.IsZero() && c.deadline.Before(t) {
		return c.deadline
	}
	return t
}

func (c *idleConn) Read(p []byte) (int, error) {
	if err := c.Conn.SetReadDeadline(c.nextDeadline()); err != nil {
		return 0, err
	}
	return c.Conn.Read(p)
}

func (c *idleConn) Write(p []byte) (int, error) {
	if err := c.Conn.SetWriteDeadline(c.nextDeadline()); err != nil {
		return 0, err
	}
	return c.Conn.Write(p)
}
