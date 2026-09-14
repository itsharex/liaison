package aigateway

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Opt-in adapter test against a local model. Production still dials connectors.
func TestOllamaLive(t *testing.T) {
	address, model := os.Getenv("OLLAMA_TEST_ADDR"), os.Getenv("OLLAMA_TEST_MODEL")
	if address == "" || model == "" {
		t.Skip("set OLLAMA_TEST_ADDR and OLLAMA_TEST_MODEL")
	}
	u, err := NewUpstream(Target{Host: "model.internal", Port: 11434, BasePath: "/api"}, func(ctx context.Context) (net.Conn, error) { return (&net.Dialer{}).DialContext(ctx, "tcp", address) })
	require.NoError(t, err)
	defer u.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	require.Equal(t, "compatible", u.Probe(ctx, "", "ollama").State)
	for _, stream := range []bool{false, true} {
		p, err := Prepare([]byte(fmt.Sprintf(`{"model":"demo","messages":[{"role":"user","content":"Reply with hello."}],"max_tokens":16,"stream":%t}`, stream)), map[string]string{"demo": model}, "ollama")
		require.NoError(t, err)
		resp, err := u.RequestProtocol(ctx, "POST", p.Operation, "", "ollama", bytes.NewReader(p.Body))
		require.NoError(t, err)
		var usage Usage
		var output bytes.Buffer
		if stream {
			err = RelaySSE(resp.Body, "ollama", "demo", func(b []byte) error { _, e := output.Write(b); return e }, &usage)
		} else {
			var raw, data []byte
			raw, err = io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			require.NoError(t, err)
			data, err = RewriteJSON(raw, "ollama", "demo", &usage)
			output.Write(data)
		}
		require.NoError(t, resp.Body.Close())
		require.NoError(t, err)
		require.True(t, usage.Complete)
		require.NotNil(t, usage.Input)
		require.NotNil(t, usage.Output)
		require.Contains(t, output.String(), `"model":"demo"`)
		t.Logf("real Ollama stream=%t input=%d output=%d", stream, *usage.Input, *usage.Output)
	}
}
