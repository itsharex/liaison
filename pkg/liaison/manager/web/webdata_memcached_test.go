package web

import (
	"context"
	"io"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMemcachedCommandClassification(t *testing.T) {
	for _, tc := range []struct {
		statement   string
		valid, read bool
	}{
		{`{"operation":"stats"}`, true, true},
		{`{"operation":"get","key":"key"}`, true, true},
		{`{"operation":"set","key":"key","value":"aGVsbG8=","ttl_seconds":60}`, true, false},
		{`{"operation":"delete","key":"key"}`, true, false},
		{`{"operation":"get","key":"key","target":"other"}`, false, false},
		{`{"operation":"get","key":"key\r\nflush_all"}`, false, false},
		{`{"operation":"stats"} {}`, false, false},
		{`{"operation":"flush_all"}`, false, false},
		{`{"operation":"set","key":"key","value":"not base64"}`, false, false},
	} {
		t.Run(tc.statement, func(t *testing.T) {
			_, err := parseMemcachedCommand(tc.statement)
			require.Equal(t, tc.valid, err == nil)
			require.Equal(t, tc.read, webDataExecuteIsQuery("memcached", tc.statement))
		})
	}
}

func TestMemcachedAuditRedactsValuesAndMalformedInput(t *testing.T) {
	for _, statement := range []string{
		`{"operation":"set","key":"private-key","value":"c2VjcmV0"}`,
		`{"operation":"get","key":"private-key"}`,
		`{"operation":"stats","unknown":"secret"}`,
		`invalid secret`,
	} {
		preview := webDataAuditPreview("memcached", statement)
		require.NotContains(t, preview, "secret")
		require.NotContains(t, preview, "c2VjcmV0")
		require.NotContains(t, preview, "private-key")
	}
	require.Equal(t, "memcached set", webDataAuditPreview("memcached", `{"operation":"set","key":"k","value":"eA=="}`))
	require.Equal(t, "SELECT 1", webDataAuditPreview("mysql", "SELECT 1"))
}

func TestMemcachedWebDataExecution(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	done := make(chan error, 1)
	go func() {
		defer server.Close()
		request := make([]byte, len("stats\r\n"))
		if _, err := io.ReadFull(server, request); err != nil {
			done <- err
			return
		}
		_, err := server.Write([]byte("STAT curr_items 2\r\nSTAT bytes 32\r\nEND\r\n"))
		done <- err
	}()
	s := &webDataSession{protocol: "memcached", cacheDial: func(context.Context) (net.Conn, error) { return client, nil }}
	result, err := s.execute(context.Background(), `{"operation":"stats"}`)
	require.NoError(t, err)
	require.Equal(t, []string{"name", "value"}, result.Columns)
	require.Equal(t, "bytes", result.Rows[0]["name"])
	require.Equal(t, "32", result.Rows[0]["value"])
	require.NoError(t, <-done)
	require.NotContains(t, webDataCapabilities("memcached"), "sql")
	require.NotContains(t, webDataCapabilities("memcached"), "key_scan")
}
