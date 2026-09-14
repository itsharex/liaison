package dbgateway

import (
	"encoding/binary"
	"testing"
)

func TestPostgresRequiresSSLRequest(t *testing.T) {
	for _, code := range []uint32{80877103, 196608, 80877102, 80877104} {
		packet := make([]byte, 8)
		binary.BigEndian.PutUint32(packet, 8)
		binary.BigEndian.PutUint32(packet[4:], code)
		if (RequirePostgresTLS(packet) == nil) != (code == 80877103) {
			t.Fatalf("code %d", code)
		}
	}
	for n := 0; n < 8; n++ {
		if RequirePostgresTLS(make([]byte, n)) == nil {
			t.Fatal("accepted truncation")
		}
	}
}

func TestMySQLRequiresSSLRequest(t *testing.T) {
	packet := make([]byte, 36)
	packet[0] = 32
	packet[3] = 1
	binary.LittleEndian.PutUint32(packet[4:], 1<<9|1<<11)
	if err := RequireMySQLTLS(packet); err != nil {
		t.Fatal(err)
	}
	for _, offset := range []int{0, 1, 2, 3, 5} {
		copyPacket := append([]byte(nil), packet...)
		copyPacket[offset] ^= 255
		if RequireMySQLTLS(copyPacket) == nil {
			t.Fatalf("accepted invalid offset %d", offset)
		}
	}
	if RequireMySQLTLS(append(packet, 0)) == nil {
		t.Fatal("accepted authentication payload")
	}
}

func FuzzNegotiation(f *testing.F) {
	f.Add([]byte{0, 0, 0, 8, 4, 210, 22, 47})
	f.Fuzz(func(t *testing.T, b []byte) {
		if RequirePostgresTLS(b) == nil && len(b) != 8 {
			t.Fatal("invalid PostgreSQL frame")
		}
		if RequireMySQLTLS(b) == nil && len(b) != 36 {
			t.Fatal("invalid MySQL frame")
		}
	})
}
