package aigateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOllamaPrepare(t *testing.T) {
	raw := `{"model":"public","messages":[{"role":"system","content":"Answer briefly"},{"role":"user","content":"Hi"}],"max_tokens":64,"temperature":0.5,"stop":"END","stream":true,"stream_options":{"include_usage":true}}`
	p, err := Prepare([]byte(raw), map[string]string{"public": "private:latest"}, "ollama")
	require.NoError(t, err)
	require.Equal(t, "chat", p.Operation)
	require.Equal(t, "public", p.Alias)
	require.True(t, p.Stream)
	var obj map[string]any
	require.NoError(t, json.Unmarshal(p.Body, &obj))
	require.Equal(t, "private:latest", obj["model"])
	require.Equal(t, float64(64), obj["options"].(map[string]any)["num_predict"])
	require.NotContains(t, obj, "stream_options")
	for _, extra := range []string{`,"tools":[]`, `,"max_tokens":-1`, `,"temperature":null`, `,"top_p":2`, `,"keep_alive":-1`, `,"response_format":{}`} {
		_, err = Prepare([]byte(`{"model":"public","messages":[{"role":"user","content":"hi"}]`+extra+`}`), map[string]string{"public": "private"}, "ollama")
		require.ErrorIs(t, err, ErrUnsupported)
	}
	_, err = Prepare([]byte(raw), nil, "ollama")
	require.ErrorIs(t, err, ErrModelDenied)
}

const ollamaFinal = `{"model":"private:latest","message":{"role":"assistant","content":"Hello","thinking":"Reason"},"done":true,"done_reason":"stop","prompt_eval_count":11,"eval_count":4}`

func TestOllamaJSONAndStream(t *testing.T) {
	var usage Usage
	data, err := RewriteJSON([]byte(ollamaFinal), "ollama", "public", &usage)
	require.NoError(t, err)
	require.True(t, usage.Complete)
	require.EqualValues(t, 11, *usage.Input)
	require.EqualValues(t, 4, *usage.Output)
	require.Contains(t, string(data), `"model":"public"`)
	require.NotContains(t, string(data), "private")
	require.Contains(t, string(data), `"reasoning_content":"Reason"`)
	stream := `{"model":"private:latest","message":{"role":"assistant","content":"Hi"},"done":false}` + "\n" + ollamaFinal + "\n"
	var output bytes.Buffer
	usage = Usage{}
	err = RelaySSE(strings.NewReader(stream), "ollama", "public", func(b []byte) error { _, err := output.Write(b); return err }, &usage)
	require.NoError(t, err)
	require.True(t, usage.Complete)
	require.Contains(t, output.String(), "[DONE]")
	require.Contains(t, output.String(), `"total_tokens":15`)
	require.NotContains(t, output.String(), "private")
}
func TestOllamaFailuresNeverComplete(t *testing.T) {
	for _, raw := range []string{`{}`, `{"error":"secret"}`, `{"model":"private","message":{"role":"assistant","content":"partial"},"done":false}`, strings.Replace(ollamaFinal, `"stop"`, `"unknown"`, 1), strings.Replace(ollamaFinal, `"eval_count":4`, `"eval_count":-1`, 1), strings.Repeat("x", (1<<20)+1)} {
		var usage Usage
		var out bytes.Buffer
		err := RelaySSE(strings.NewReader(raw), "ollama", "public", func(b []byte) error { _, err := out.Write(b); return err }, &usage)
		require.Error(t, err)
		require.False(t, usage.Complete)
		require.NotContains(t, out.String(), "[DONE]")
		require.NotContains(t, out.String(), "secret")
	}
	var usage Usage
	err := RelaySSE(strings.NewReader(ollamaFinal), "ollama", "public", func([]byte) error { return io.ErrClosedPipe }, &usage)
	require.True(t, errors.Is(err, io.ErrClosedPipe))
	require.False(t, usage.Complete)
	_, err = RewriteJSON([]byte(strings.ReplaceAll(strings.ReplaceAll(ollamaFinal, `,"prompt_eval_count":11`, ""), `,"eval_count":4`, "")), "ollama", "public", &usage)
	require.NoError(t, err)
	require.Nil(t, usage.Input)
	require.Nil(t, usage.Output)
}
func TestOllamaProbeAndRestrictedOperations(t *testing.T) {
	u := testUpstream(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/tags", r.URL.Path)
		require.Empty(t, r.Header.Get("Authorization"))
		require.Empty(t, r.Header.Get("Cookie"))
		_, err := io.WriteString(w, `{"models":[{"name":"qwen:latest"},{"name":"qwen:latest"}]}`)
		require.NoError(t, err)
	})
	result := u.Probe(context.Background(), "", "ollama")
	require.Equal(t, "compatible", result.State)
	require.Equal(t, []string{"qwen:latest"}, result.Models)
	for _, op := range []string{"pull", "push", "delete", "create", "generate", "../tags"} {
		_, err := u.RequestProtocol(context.Background(), "POST", op, "", "ollama", nil)
		require.Error(t, err)
	}
}
