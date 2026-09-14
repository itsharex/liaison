package web

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"sort"
	"strings"

	"github.com/liaisonio/liaison/pkg/liaison/manager/cacheaccess"
)

func parseMemcachedCommand(statement string) (cacheaccess.Command, error) {
	var input struct {
		Operation  string `json:"operation"`
		Key        string `json:"key"`
		Value      []byte `json:"value"`
		Flags      uint32 `json:"flags"`
		TTLSeconds uint32 `json:"ttl_seconds"`
	}
	if len(statement) > webDataStatementMaxSize {
		return cacheaccess.Command{}, cacheaccess.ErrInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(statement))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return cacheaccess.Command{}, cacheaccess.ErrInvalid
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return cacheaccess.Command{}, cacheaccess.ErrInvalid
	}
	cmd := cacheaccess.Command{Operation: input.Operation, Key: input.Key, Value: input.Value, Flags: input.Flags, TTLSeconds: input.TTLSeconds}
	return cmd, cmd.Validate()
}

func memcachedIsQuery(statement string) bool {
	cmd, err := parseMemcachedCommand(statement)
	return err == nil && (cmd.Operation == "get" || cmd.Operation == "stats")
}

// Cache values may contain tokens or personal data. Audit only a validated
// operation name; even malformed input must never fall back to raw text.
func webDataAuditPreview(protocol, statement string) string {
	if protocol == "s3" {
		command, err := parseStorageCommand(statement)
		if err != nil {
			return "s3 operation"
		}
		return "s3 " + command.Operation
	}
	if protocol != "memcached" {
		return webDataStatementPreview(statement)
	}
	cmd, err := parseMemcachedCommand(statement)
	if err != nil {
		return "memcached operation"
	}
	return "memcached " + cmd.Operation
}

func (web *web) openWebDataMemcached(ctx context.Context, s *webDataSession, password string) error {
	// Basic-text Memcached is deliberately not presented as SASL support.
	if s.username != "" || password != "" || s.database != "" || s.schema != "" || s.connectionParams != "" || s.authMechanism != "" {
		return errors.New("Memcached basic-text access does not support these credentials or connection parameters")
	}
	if s.tlsMode != "" && s.tlsMode != "disable" && s.tlsMode != "require" && s.tlsMode != "skip-verify" {
		return errors.New("unsupported Memcached TLS mode")
	}
	s.cacheDial = func(ctx context.Context) (net.Conn, error) {
		// Revalidate ownership/access on EVERY operation, not only session creation.
		conn, target, err := web.controlPlane.OpenWebDataStream(ctx, s.proxyID)
		if err != nil {
			return nil, err
		}
		if target.Protocol != "memcached" {
			conn.Close()
			return nil, errors.New("cache target protocol changed")
		}
		if s.tlsMode == "require" || s.tlsMode == "skip-verify" {
			secured := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: target.TargetHost, InsecureSkipVerify: s.tlsMode == "skip-verify"}) // Explicit existing TLS option.
			if err := secured.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, errors.New("cache TLS handshake failed")
			}
			return secured, nil
		}
		return conn, nil
	}
	_, err := s.executeMemcached(ctx, `{"operation":"stats"}`)
	if err != nil {
		s.cacheDial = nil
	}
	return err
}

func (s *webDataSession) executeMemcached(ctx context.Context, statement string) (*webDataExecuteResponse, error) {
	cmd, err := parseMemcachedCommand(statement)
	if err != nil {
		return nil, err
	}
	if s.cacheDial == nil {
		return nil, errors.New("cache session is not connected")
	}
	conn, err := s.cacheDial(ctx)
	if err != nil {
		return nil, errors.New("cache connection unavailable")
	}
	// This method is invoked after WebData authorization. Agent write approval
	// must classify set/delete as mutations before invoking this executor.
	result, err := cacheaccess.Execute(ctx, conn, cmd, true)
	if err != nil {
		return nil, errors.New("cache operation failed")
	}
	switch cmd.Operation {
	case "stats":
		keys := make([]string, 0, len(result.Stats))
		for key := range result.Stats {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		rows := make([]map[string]any, 0, len(keys))
		for _, key := range keys {
			rows = append(rows, map[string]any{"name": key, "value": result.Stats[key]})
		}
		return &webDataExecuteResponse{Type: "rows", Columns: []string{"name", "value"}, Rows: rows}, nil
	case "get":
		rows := []map[string]any{}
		if result.Found {
			rows = append(rows, map[string]any{"key": cmd.Key, "flags": result.Flags, "value": result.Value})
		}
		return &webDataExecuteResponse{Type: "rows", Columns: []string{"key", "flags", "value"}, Rows: rows}, nil
	default:
		affected := int64(1)
		if result.Status == "NOT_FOUND" {
			affected = 0
		}
		return &webDataExecuteResponse{Type: "message", Message: result.Status, AffectedRows: affected}, nil
	}
}
