// Package cacheaccess provides bounded cache operations over an authorized
// connector stream. It never opens a network connection itself.
package cacheaccess

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const maxValueBytes = 1 << 20

var ErrInvalid = errors.New("invalid cache operation")
var ErrResponse = errors.New("invalid cache response")
var ErrWriteDenied = errors.New("cache write requires authorization")

type Command struct {
	Operation  string
	Key        string
	Value      []byte
	Flags      uint32
	TTLSeconds uint32
}

type Result struct {
	Found  bool
	Value  []byte
	Flags  uint32
	Stats  map[string]string
	Status string
}

func (c Command) Validate() error {
	switch c.Operation {
	case "stats":
		if c.Key != "" || len(c.Value) != 0 || c.Flags != 0 || c.TTLSeconds != 0 {
			return ErrInvalid
		}
		return nil
	case "get", "delete":
		if len(c.Value) != 0 || c.Flags != 0 || c.TTLSeconds != 0 {
			return ErrInvalid
		}
	case "set":
		// Use relative expiry only; avoid accidental Unix-timestamp semantics.
		if len(c.Value) > maxValueBytes || c.TTLSeconds > 30*24*60*60 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	if len(c.Key) == 0 || len(c.Key) > 250 {
		return ErrInvalid
	}
	for _, b := range []byte(c.Key) {
		if b <= 32 || b == 127 {
			return ErrInvalid
		}
	}
	return nil
}

// Execute owns and closes conn even on validation failure. The caller supplies
// a fresh, authorized connector stream (optionally TLS-wrapped) per operation.
// This avoids reusing a stream after cancellation or malformed framing.
func Execute(ctx context.Context, conn net.Conn, cmd Command, allowWrite bool) (Result, error) {
	var result Result
	if conn == nil {
		return result, ErrInvalid
	}
	defer conn.Close() // Best-effort cleanup of the owned stream.
	if err := cmd.Validate(); err != nil {
		return result, err
	}
	if (cmd.Operation == "set" || cmd.Operation == "delete") && !allowWrite {
		return result, ErrWriteDenied
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	// Connector streams are not OS sockets. Bound the whole operation through
	// context and Close (which unblocks IO), not their optional deadline API.
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	writer := bufio.NewWriter(conn)
	var err error
	switch cmd.Operation {
	case "stats":
		_, err = writer.WriteString("stats\r\n")
	case "get", "delete":
		_, err = fmt.Fprintf(writer, "%s %s\r\n", cmd.Operation, cmd.Key)
	case "set":
		_, err = fmt.Fprintf(writer, "set %s %d %d %d\r\n", cmd.Key, cmd.Flags, cmd.TTLSeconds, len(cmd.Value))
		if err == nil {
			_, err = writer.Write(cmd.Value)
		}
		if err == nil {
			_, err = writer.WriteString("\r\n")
		}
	}
	if err != nil {
		return result, err
	}
	if err := writer.Flush(); err != nil {
		return result, err
	}
	reader := bufio.NewReaderSize(conn, 4096)
	line, err := readLine(reader)
	if err != nil {
		return result, err
	}
	switch cmd.Operation {
	case "get":
		if line == "END" {
			return result, nil
		}
		parts := strings.Split(line, " ")
		if len(parts) != 4 || parts[0] != "VALUE" || parts[1] != cmd.Key {
			return result, ErrResponse
		}
		flags, err := strconv.ParseUint(parts[2], 10, 32)
		if err != nil {
			return result, ErrResponse
		}
		size, err := strconv.ParseUint(parts[3], 10, 32)
		if err != nil || size > maxValueBytes {
			return result, ErrResponse
		}
		value := make([]byte, int(size)+2)
		if _, err := io.ReadFull(reader, value); err != nil {
			return result, err
		}
		if string(value[size:]) != "\r\n" {
			return result, ErrResponse
		}
		end, err := readLine(reader)
		if err != nil {
			return result, err
		}
		if end != "END" {
			return result, ErrResponse
		}
		return Result{Found: true, Value: value[:size], Flags: uint32(flags)}, nil
	case "stats":
		stats := make(map[string]string)
		total := 0
		for line != "END" {
			total += len(line)
			parts := strings.SplitN(line, " ", 3)
			if len(parts) != 3 || parts[0] != "STAT" || parts[1] == "" || len(stats) >= 1024 || total > 256<<10 {
				return result, ErrResponse
			}
			if _, exists := stats[parts[1]]; exists {
				return result, ErrResponse
			}
			stats[parts[1]] = parts[2]
			line, err = readLine(reader)
			if err != nil {
				return result, err
			}
		}
		return Result{Stats: stats}, nil
	case "set":
		if line != "STORED" {
			return result, ErrResponse
		}
	case "delete":
		if line != "DELETED" && line != "NOT_FOUND" {
			return result, ErrResponse
		}
	}
	return Result{Status: line}, nil
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadSlice('\n')
	if err != nil {
		return "", ErrResponse
	}
	if len(line) < 2 || line[len(line)-2] != '\r' {
		return "", ErrResponse
	}
	return string(line[:len(line)-2]), nil
}
