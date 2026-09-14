package aigateway

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"math"
)

// PrepareMessages preserves native Anthropic content (including tools) rather
// than converting it through the text-only OpenAI adapter. Only the model alias
// is replaced. This adapter must only be used with an Anthropic upstream.
func PrepareMessages(raw []byte, allowed map[string]string) (Prepared, error) {
	var p Prepared
	var obj map[string]json.RawMessage
	if len(raw) > 1<<20 || json.Unmarshal(raw, &obj) != nil || obj == nil {
		return p, ErrUnsupported
	}
	for field := range obj {
		switch field {
		case "model", "messages", "max_tokens", "stream", "system", "temperature", "top_p", "top_k", "stop_sequences", "tools", "tool_choice", "thinking", "metadata":
		default:
			return p, ErrUnsupported
		}
	}
	if json.Unmarshal(obj["model"], &p.Alias) != nil {
		return p, ErrUnsupported
	}
	model, ok := allowed[p.Alias]
	if !ok {
		return p, ErrModelDenied
	}
	var maxTokens int64
	if json.Unmarshal(obj["max_tokens"], &maxTokens) != nil || maxTokens <= 0 {
		return p, ErrUnsupported
	}
	if v, ok := obj["stream"]; ok && (bytes.Equal(v, []byte("null")) || json.Unmarshal(v, &p.Stream) != nil) {
		return p, ErrUnsupported
	}
	var messages []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(obj["messages"], &messages) != nil || len(messages) == 0 || len(messages) > 1000 {
		return p, ErrUnsupported
	}
	for _, message := range messages {
		if message.Role != "user" && message.Role != "assistant" || len(message.Content) == 0 || bytes.Equal(message.Content, []byte("null")) {
			return p, ErrUnsupported
		}
		var text string
		var blocks []json.RawMessage
		if json.Unmarshal(message.Content, &text) != nil && (json.Unmarshal(message.Content, &blocks) != nil || len(blocks) == 0) {
			return p, ErrUnsupported
		}
	}
	var err error
	obj["model"], err = json.Marshal(model)
	if err != nil {
		return p, err
	}
	p.Body, err = json.Marshal(obj)
	p.Operation = "messages"
	return p, err
}

// RewriteMessagesJSON leaves native content blocks intact and never exposes the
// internal model identifier. Missing usage remains unknown rather than zero.
func RewriteMessagesJSON(raw []byte, alias string, u *Usage) ([]byte, error) {
	obj, err := nativeMessage(raw, alias, u)
	if err != nil {
		return nil, err
	}
	var reason string
	if json.Unmarshal(obj["stop_reason"], &reason) != nil || reason == "" {
		return nil, ErrResponse
	}
	out, err := json.Marshal(obj)
	if err == nil {
		u.Complete = true
	}
	return out, err
}

func nativeMessage(raw []byte, alias string, u *Usage) (map[string]json.RawMessage, error) {
	obj, err := responseObject(raw)
	if err != nil {
		return nil, err
	}
	var kind, role, id string
	var content []json.RawMessage
	if json.Unmarshal(obj["type"], &kind) != nil || kind != "message" || json.Unmarshal(obj["role"], &role) != nil || role != "assistant" || json.Unmarshal(obj["id"], &id) != nil || id == "" || json.Unmarshal(obj["content"], &content) != nil || content == nil {
		return nil, ErrResponse
	}
	if err := nativeUsage(obj, u); err != nil {
		return nil, err
	}
	obj["model"], err = json.Marshal(alias)
	return obj, err
}

// RelayMessagesSSE preserves native tool/thinking deltas. Completion requires
// message_delta followed by message_stop; EOF or provider errors are failures.
func RelayMessagesSSE(reader io.Reader, alias string, emit func([]byte) error, u *Usage) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	var data bytes.Buffer
	started, ended := false, false
	dispatch := func() error {
		if data.Len() == 0 {
			return nil
		}
		obj, err := responseObject(bytes.TrimSpace(data.Bytes()))
		if err != nil {
			return err
		}
		var kind string
		if json.Unmarshal(obj["type"], &kind) != nil {
			return ErrResponse
		}
		switch kind {
		case "ping":
		case "message_start":
			if started {
				return ErrResponse
			}
			message, err := nativeMessage(obj["message"], alias, u)
			if err != nil {
				return err
			}
			obj["message"], err = json.Marshal(message)
			if err != nil {
				return err
			}
			started = true
		case "content_block_start", "content_block_delta", "content_block_stop":
			if !started || ended {
				return ErrResponse
			}
		case "message_delta":
			if !started {
				return ErrResponse
			}
			var delta struct {
				StopReason *string `json:"stop_reason"`
			}
			if json.Unmarshal(obj["delta"], &delta) != nil {
				return ErrResponse
			}
			if delta.StopReason != nil && *delta.StopReason != "" {
				ended = true
			}
			if err := nativeUsage(obj, u); err != nil {
				return err
			}
		case "message_stop":
			if !started || !ended {
				return ErrResponse
			}
		default:
			return ErrResponse
		}
		raw, err := json.Marshal(obj)
		if err != nil {
			return err
		}
		if err = emit(append(append([]byte("event: "+kind+"\ndata: "), raw...), '\n', '\n')); err != nil {
			return err
		}
		if kind == "message_stop" {
			u.Complete = true
		}
		return nil
	}
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if err := dispatch(); err != nil {
				return err
			}
			if u.Complete {
				return nil
			}
			data.Reset()
		} else if bytes.HasPrefix(line, []byte("data:")) {
			if data.Len()+len(line) > 1<<20 {
				return ErrResponse
			}
			data.Write(bytes.TrimPrefix(line, []byte("data:")))
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return ErrResponse
}

// Anthropic reports cache tokens separately. Count them toward the same input
// budget, and reject malformed counters instead of treating them as free usage.
func nativeUsage(obj map[string]json.RawMessage, u *Usage) error {
	if obj["usage"] == nil {
		return nil
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(obj["usage"], &values) != nil || values == nil {
		return ErrResponse
	}
	var input int64
	hasInput := false
	for _, key := range []string{"input_tokens", "cache_read_input_tokens", "cache_creation_input_tokens"} {
		if raw, ok := values[key]; ok {
			var n *int64
			if json.Unmarshal(raw, &n) != nil || n == nil || *n < 0 || input > math.MaxInt64-*n {
				return ErrResponse
			}
			input += *n
			if key == "input_tokens" {
				hasInput = true
			}
		}
	}
	if hasInput {
		u.Input = &input
	}
	if raw, ok := values["output_tokens"]; ok {
		var n *int64
		if json.Unmarshal(raw, &n) != nil || n == nil || *n < 0 {
			return ErrResponse
		}
		u.Output = n
	}
	return nil
}
