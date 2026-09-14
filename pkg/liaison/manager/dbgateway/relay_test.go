package dbgateway

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func TestRelayTLSRoundTripAndCancellation(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), DNSNames: []string{"database.test"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	for _, protocol := range []string{"postgresql", "mysql"} {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			client, left := net.Pipe()
			right, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			stop := context.AfterFunc(ctx, func() { client.Close(); server.Close() })
			defer stop()
			done := make(chan error, 1)
			go func() { done <- Relay(ctx, left, right, protocol) }()
			serverDone := make(chan error, 1)
			go func() {
				if protocol == "mysql" {
					payload := append([]byte{10}, []byte("8.0-test\x00")...)
					payload = append(payload, make([]byte, 15)...)
					binary.LittleEndian.PutUint16(payload[len(payload)-2:], 1<<9|1<<11)
					if err := writePacket(server, append([]byte{byte(len(payload)), 0, 0, 0}, payload...)); err != nil {
						serverDone <- err
						return
					}
				}
				n := 8
				if protocol == "mysql" {
					n = 36
				}
				packet := make([]byte, n)
				if _, err := io.ReadFull(server, packet); err != nil {
					serverDone <- err
					return
				}
				if protocol == "postgresql" {
					if err := writePacket(server, []byte{'S'}); err != nil {
						serverDone <- err
						return
					}
				}
				tlsServer := tls.Server(server, &tls.Config{Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: key}}, MinVersion: tls.VersionTLS12})
				if err := tlsServer.HandshakeContext(ctx); err != nil {
					serverDone <- err
					return
				}
				buffer := make([]byte, 4)
				if _, err := io.ReadFull(tlsServer, buffer); err != nil {
					serverDone <- err
					return
				}
				serverDone <- writePacket(tlsServer, buffer)
			}()
			if protocol == "postgresql" {
				packet := make([]byte, 8)
				binary.BigEndian.PutUint32(packet, 8)
				binary.BigEndian.PutUint32(packet[4:], 80877103)
				if err := writePacket(client, packet); err != nil {
					t.Fatal(err)
				}
				response := make([]byte, 1)
				if _, err := io.ReadFull(client, response); err != nil || response[0] != 'S' {
					t.Fatal("SSL response", err)
				}
			} else {
				header := make([]byte, 4)
				if _, err := io.ReadFull(client, header); err != nil {
					t.Fatal(err)
				}
				if _, err := io.ReadFull(client, make([]byte, int(header[0]))); err != nil {
					t.Fatal(err)
				}
				packet := make([]byte, 36)
				packet[0] = 32
				packet[3] = 1
				binary.LittleEndian.PutUint32(packet[4:], 1<<9|1<<11)
				if err := writePacket(client, packet); err != nil {
					t.Fatal(err)
				}
			}
			tlsClient := tls.Client(client, &tls.Config{RootCAs: roots, ServerName: "database.test", MinVersion: tls.VersionTLS12})
			if err := tlsClient.HandshakeContext(ctx); err != nil {
				t.Fatal(err)
			}
			if err := writePacket(tlsClient, []byte("ping")); err != nil {
				t.Fatal(err)
			}
			buffer := make([]byte, 4)
			if _, err := io.ReadFull(tlsClient, buffer); err != nil || string(buffer) != "ping" {
				t.Fatal("round trip", err)
			}
			if err := <-serverDone; err != nil {
				t.Fatal(err)
			}
			cancel()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("relay did not stop")
			}
		})
	}
}

func TestRelayRejectsPlaintextWithoutForwarding(t *testing.T) {
	client, left := net.Pipe()
	right, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	done := make(chan error, 1)
	go func() { done <- Relay(context.Background(), left, right, "postgresql") }()
	packet := make([]byte, 8)
	binary.BigEndian.PutUint32(packet, 8)
	binary.BigEndian.PutUint32(packet[4:], 196608)
	if err := writePacket(client, packet); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != ErrNegotiation {
		t.Fatal(err)
	}
	buffer := make([]byte, 1)
	if n, err := server.Read(buffer); n != 0 || err != io.EOF {
		t.Fatal("forwarded plaintext")
	}
}
