package dbgateway

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

// Relay owns both already-authorized connections. The caller must obtain the
// upstream through the connector, never from a client-supplied address. TLS and
// database account authentication remain end-to-end between client and database.
// Source policy, lifecycle revocation and traffic accounting belong to the caller.
func Relay(ctx context.Context, client, upstream net.Conn, protocol string) error {
	defer client.Close() // Best-effort cleanup of owned connections.
	defer upstream.Close()
	stop := context.AfterFunc(ctx, func() { client.Close(); upstream.Close() })
	defer stop()
	deadline := time.Now().Add(10 * time.Second)
	if err := client.SetDeadline(deadline); err != nil {
		return err
	}
	if err := upstream.SetDeadline(deadline); err != nil {
		return err
	}
	switch protocol {
	case "postgresql":
		request := make([]byte, 8)
		if _, err := io.ReadFull(client, request); err != nil {
			return err
		}
		if err := RequirePostgresTLS(request); err != nil {
			return err
		}
		if err := writePacket(upstream, request); err != nil {
			return err
		}
		response := make([]byte, 1)
		if _, err := io.ReadFull(upstream, response); err != nil {
			return err
		}
		if response[0] != 'S' {
			return ErrNegotiation
		}
		if err := writePacket(client, response); err != nil {
			return err
		}
	case "mysql":
		header := make([]byte, 4)
		if _, err := io.ReadFull(upstream, header); err != nil {
			return err
		}
		size := int(header[0]) | int(header[1])<<8 | int(header[2])<<16
		if header[3] != 0 || size < 1 || size > 4096 {
			return ErrNegotiation
		}
		greeting := make([]byte, size)
		if _, err := io.ReadFull(upstream, greeting); err != nil {
			return err
		}
		if err := mysqlGreetingTLS(greeting); err != nil {
			return err
		}
		if err := writePacket(client, append(header, greeting...)); err != nil {
			return err
		}
		request := make([]byte, 36)
		if _, err := io.ReadFull(client, request); err != nil {
			return err
		}
		if err := RequireMySQLTLS(request); err != nil {
			return err
		}
		if err := writePacket(upstream, request); err != nil {
			return err
		}
	default:
		return ErrNegotiation
	}
	// Refuse a plaintext startup/authentication packet after SSLRequest.
	header := make([]byte, 5)
	if _, err := io.ReadFull(client, header); err != nil {
		return err
	}
	if header[0] != 22 || header[1] != 3 || header[2] > 3 || binary.BigEndian.Uint16(header[3:]) == 0 || binary.BigEndian.Uint16(header[3:]) > 18432 {
		return ErrNegotiation
	}
	if err := writePacket(upstream, header); err != nil {
		return err
	}
	if err := client.SetDeadline(time.Time{}); err != nil {
		return err
	}
	if err := upstream.SetDeadline(time.Time{}); err != nil {
		return err
	}
	done := make(chan error, 2)
	copyStream := func(dst, src net.Conn) {
		var err error
		defer func() {
			if recover() != nil {
				err = errors.New("database relay failed")
			}
			done <- err
		}()
		_, err = io.Copy(dst, src)
	}
	go copyStream(upstream, client)
	go copyStream(client, upstream)
	err := <-done
	client.Close() // Unblock the other direction on EOF/error; no dangling session.
	upstream.Close()
	<-done
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func mysqlGreetingTLS(payload []byte) error {
	if len(payload) < 2 || payload[0] != 10 {
		return ErrNegotiation
	}
	end := bytes.IndexByte(payload[1:], 0)
	if end <= 0 {
		return ErrNegotiation
	}
	// Version terminator, connection ID, salt, filler, then lower capability bits.
	capability := 1 + end + 1 + 4 + 8 + 1
	if len(payload) < capability+2 {
		return ErrNegotiation
	}
	if binary.LittleEndian.Uint16(payload[capability:])&(1<<9|1<<11) != (1<<9 | 1<<11) {
		return ErrNegotiation
	}
	return nil
}

func writePacket(w io.Writer, data []byte) error {
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
