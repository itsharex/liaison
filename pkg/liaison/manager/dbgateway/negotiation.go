// Package dbgateway contains native database gateway negotiation primitives.
// These parsers are not authorization and must run only after access policy
// checks. They neither terminate TLS nor log/decode database credentials.
package dbgateway

import (
	"encoding/binary"
	"errors"
)

var ErrNegotiation = errors.New("unsupported database encryption negotiation")

// RequirePostgresTLS accepts only the standard eight-byte SSLRequest. Reject
// cleartext StartupMessage, GSS negotiation, and unauthenticated CancelRequest.
// A gateway must relay the server's S response, never synthesize TLS support.
func RequirePostgresTLS(packet []byte) error {
	if len(packet) != 8 || binary.BigEndian.Uint32(packet[:4]) != 8 || binary.BigEndian.Uint32(packet[4:]) != 80877103 {
		return ErrNegotiation
	}
	return nil
}

// RequireMySQLTLS accepts the first client SSLRequest (32-byte payload, packet
// sequence 1), not a plaintext HandshakeResponse carrying authentication data.
// The caller must also validate the upstream greeting advertises CLIENT_SSL.
func RequireMySQLTLS(packet []byte) error {
	if len(packet) != 36 || packet[0] != 32 || packet[1] != 0 || packet[2] != 0 || packet[3] != 1 {
		return ErrNegotiation
	}
	const clientProtocol41 = 1 << 9
	const clientSSL = 1 << 11
	flags := binary.LittleEndian.Uint32(packet[4:8])
	if flags&(clientProtocol41|clientSSL) != clientProtocol41|clientSSL {
		return ErrNegotiation
	}
	return nil
}
