package aigateway

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNativeMessagesCacheUsage(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{`{"input_tokens":4,"cache_read_input_tokens":6,"cache_creation_input_tokens":2,"output_tokens":3}`, true},
		{`{"input_tokens":-1}`, false}, {`{"output_tokens":null}`, false},
		{`{"input_tokens":9223372036854775807,"cache_read_input_tokens":1}`, false},
	} {
		u := Usage{}
		err := nativeUsage(map[string]json.RawMessage{"usage": json.RawMessage(tc.raw)}, &u)
		if (err == nil) != tc.valid {
			t.Fatalf("usage validation: %v", err)
		}
		if tc.valid && (*u.Input != 12 || *u.Output != 3) {
			t.Fatal("cache tokens not counted")
		}
	}
}

func TestPrepareMessagesPreservesToolsAndRestrictsModels(t *testing.T) {
	raw := []byte(`{"model":"public","max_tokens":20,"messages":[{"role":"user","content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}],"tools":[{"name":"lookup","input_schema":{"type":"object"}}]}`)
	p, err := PrepareMessages(raw, map[string]string{"public": "private"})
	if err != nil || p.Operation != "messages" || !bytes.Contains(p.Body, []byte(`"model":"private"`)) || !bytes.Contains(p.Body, []byte(`"tool_result"`)) {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := PrepareMessages(raw, nil); !errors.Is(err, ErrModelDenied) {
		t.Fatal(err)
	}
	for _, invalid := range []string{`{}`, `{"model":"public","max_tokens":0,"messages":[]}`, `{"model":"public","max_tokens":1,"messages":[{"role":"system","content":"x"}]}`, `{"model":"public","max_tokens":1,"stream":null,"messages":[{"role":"user","content":"x"}]}`} {
		if _, err := PrepareMessages([]byte(invalid), map[string]string{"public": "private"}); err == nil {
			t.Fatal("accepted invalid request")
		}
	}
}

func TestNativeMessagesResponseAndStream(t *testing.T) {
	message := `{"type":"message","id":"msg_test","role":"assistant","model":"private","content":[],"stop_reason":"tool_use","usage":{"input_tokens":4,"output_tokens":2}}`
	u := Usage{}
	out, err := RewriteMessagesJSON([]byte(message), "public", &u)
	if err != nil || bytes.Contains(out, []byte("private")) || !u.Complete || *u.Input != 4 || *u.Output != 2 {
		t.Fatalf("response: %v %+v", err, u)
	}
	start := "event: message_start\ndata: {\"type\":\"message_start\",\"message\":" + message + "}\n\n"
	tool := "data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{}\"}}\n\n"
	delta := "data: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"tool_use\"},\"usage\":{\"output_tokens\":6}}\n\n"
	stop := "data: {\"type\":\"message_stop\"}\n\n"
	for _, tc := range []struct {
		name, stream string
		valid        bool
	}{
		{"complete", start + tool + delta + stop, true}, {"truncated", start + tool + delta, false}, {"missing delta", start + stop, false}, {"no start", stop, false}, {"provider error", start + "data: {\"type\":\"error\",\"error\":{\"message\":\"secret\"}}\n\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			u := Usage{}
			var output bytes.Buffer
			err := RelayMessagesSSE(strings.NewReader(tc.stream), "public", func(b []byte) error { _, err := output.Write(b); return err }, &u)
			if (err == nil) != tc.valid || u.Complete != tc.valid || strings.Contains(output.String(), "private") || strings.Contains(output.String(), "secret") {
				t.Fatalf("stream: %v %+v", err, u)
			}
			if tc.valid && (*u.Input != 4 || *u.Output != 6 || !strings.Contains(output.String(), "input_json_delta")) {
				t.Fatal("lost tools or cumulative usage")
			}
		})
	}
	u = Usage{}
	if err := RelayMessagesSSE(strings.NewReader(start+delta+stop), "public", func([]byte) error { return errors.New("closed") }, &u); err == nil || u.Complete {
		t.Fatal("ignored write failure")
	}
}
