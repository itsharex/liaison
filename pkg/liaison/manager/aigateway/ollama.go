package aigateway

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"time"
)

// prepareOllama converts the supported text-chat subset without silently dropping
// tools, images or provider options. Model authorization has already run.
func prepareOllama(p Prepared, obj map[string]json.RawMessage) (Prepared, error) {
	options := map[string]json.RawMessage{}
	for field, raw := range obj {
		switch field {
		case "model", "messages", "stream":
		case "temperature", "top_p":
			var n *float64
			if json.Unmarshal(raw, &n) != nil || n == nil || *n < 0 || (field == "top_p" && *n > 1) || (field == "temperature" && *n > 2) {
				return p, ErrUnsupported
			}
			options[field] = raw
		case "max_tokens", "seed":
			var n *int64
			if json.Unmarshal(raw, &n) != nil || n == nil || (field == "max_tokens" && (*n < 1 || *n > 32768)) {
				return p, ErrUnsupported
			}
			key := field
			if field == "max_tokens" {
				key = "num_predict"
			}
			options[key] = raw
		case "stop":
			var stops []string
			var stop string
			if json.Unmarshal(raw, &stop) == nil {
				stops = []string{stop}
			} else if json.Unmarshal(raw, &stops) != nil || stops == nil {
				return p, ErrUnsupported
			}
			encoded, err := json.Marshal(stops)
			if err != nil {
				return p, err
			}
			options[field] = encoded
		case "stream_options":
			var opts struct {
				IncludeUsage *bool `json:"include_usage"`
			}
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			if !p.Stream || decoder.Decode(&opts) != nil || opts.IncludeUsage == nil {
				return p, ErrUnsupported
			}
		default:
			return p, ErrUnsupported
		}
	}
	var messages []json.RawMessage
	if json.Unmarshal(obj["messages"], &messages) != nil {
		return p, ErrUnsupported
	}
	for _, raw := range messages {
		var msg struct {
			Role    string  `json:"role"`
			Content *string `json:"content"`
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&msg) != nil || msg.Content == nil || (msg.Role != "system" && msg.Role != "user" && msg.Role != "assistant") {
			return p, ErrUnsupported
		}
	}
	p.Operation = "chat"
	var err error
	p.Body, err = json.Marshal(map[string]any{"model": obj["model"], "messages": messages, "stream": p.Stream, "options": options})
	return p, err
}

type ollamaChat struct {
	Model   string `json:"model"`
	Message *struct {
		Role      string            `json:"role"`
		Content   string            `json:"content"`
		Thinking  string            `json:"thinking"`
		ToolCalls []json.RawMessage `json:"tool_calls"`
		Images    []json.RawMessage `json:"images"`
	} `json:"message"`
	Done   *bool  `json:"done"`
	Reason string `json:"done_reason"`
	Input  *int64 `json:"prompt_eval_count"`
	Output *int64 `json:"eval_count"`
}

func parseOllama(raw []byte) (ollamaChat, error) {
	var msg ollamaChat
	if _, err := responseObject(raw); err != nil {
		return msg, err
	}
	if json.Unmarshal(raw, &msg) != nil || msg.Done == nil || msg.Model == "" || msg.Message == nil || msg.Message.Role != "assistant" || len(msg.Message.ToolCalls) > 0 || len(msg.Message.Images) > 0 {
		return msg, ErrResponse
	}
	if *msg.Done && msg.Reason != "stop" && msg.Reason != "length" {
		return msg, ErrResponse
	}
	if msg.Input != nil && *msg.Input < 0 || msg.Output != nil && *msg.Output < 0 {
		return msg, ErrResponse
	}
	return msg, nil
}
func ollamaPayload(msg ollamaChat, alias, id string, created int64, stream bool, u *Usage) ([]byte, error) {
	message := map[string]string{"role": "assistant", "content": msg.Message.Content}
	if msg.Message.Thinking != "" {
		message["reasoning_content"] = msg.Message.Thinking
	}
	var reason any
	if *msg.Done {
		reason = msg.Reason
		u.Input = msg.Input
		u.Output = msg.Output
	}
	choice := map[string]any{"index": 0, "message": message, "finish_reason": reason}
	kind := "chat.completion"
	if stream {
		kind += ".chunk"
		delete(choice, "message")
		choice["delta"] = message
	}
	result := map[string]any{"id": id, "object": kind, "created": created, "model": alias, "choices": []any{choice}}
	if *msg.Done && u.Input != nil && u.Output != nil {
		// Avoid overflowing provider-controlled counters.
		if *u.Input > int64(^uint64(0)>>1)-*u.Output {
			return nil, ErrResponse
		}
		result["usage"] = map[string]int64{"prompt_tokens": *u.Input, "completion_tokens": *u.Output, "total_tokens": *u.Input + *u.Output}
	}
	return json.Marshal(result)
}
func rewriteOllamaJSON(raw []byte, alias string, u *Usage) ([]byte, error) {
	msg, err := parseOllama(raw)
	if err != nil {
		return nil, err
	}
	if !*msg.Done {
		return nil, ErrResponse
	}
	id, err := ollamaCompletionID()
	if err != nil {
		return nil, err
	}
	data, err := ollamaPayload(msg, alias, id, time.Now().Unix(), false, u)
	u.Complete = err == nil
	return data, err
}

// relayOllama translates bounded NDJSON records into SSE. EOF without done:true
// is a failure; unknown accounting remains unknown rather than becoming zero.
func relayOllama(reader io.Reader, alias string, emit func([]byte) error, u *Usage) error {
	id, err := ollamaCompletionID()
	if err != nil {
		return err
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	created := time.Now().Unix()
	for scanner.Scan() {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		msg, err := parseOllama(raw)
		if err != nil {
			return err
		}
		data, err := ollamaPayload(msg, alias, id, created, true, u)
		if err != nil {
			return err
		}
		if err = emit(append(append([]byte("data: "), data...), []byte("\n\n")...)); err != nil {
			return err
		}
		if *msg.Done {
			if err = emit([]byte("data: [DONE]\n\n")); err != nil {
				return err
			}
			u.Complete = true
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return ErrResponse
}

func ollamaCompletionID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return "chatcmpl-" + hex.EncodeToString(id[:]), nil
}
