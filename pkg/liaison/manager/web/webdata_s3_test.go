package web

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/manager/objectaccess"
)

func TestStorageCommandReadOnly(t *testing.T) {
	for _, statement := range []string{`{"operation":"delete"}`, `{"operation":"upload"}`, `{"operation":"list_objects","endpoint":"http://other"}`, `{"operation":"list_buckets"} {}`, `{"operation":"list_buckets","bucket":"other"}`, `null`} {
		t.Run(statement, func(t *testing.T) {
			if _, err := parseStorageCommand(statement); err == nil {
				t.Fatal("accepted invalid command")
			}
		})
	}
	for _, statement := range []string{`{"operation":"list_buckets"}`, `{"operation":"list_objects","bucket":"demo","prefix":"a//../","continuation_token":"opaque+/="}`} {
		if _, err := parseStorageCommand(statement); err != nil {
			t.Fatal(err)
		}
	}
}

func TestStorageSessionBucketScope(t *testing.T) {
	client, err := objectaccess.New(objectaccess.Config{Endpoint: "http://storage.example:9000", AccessKey: "fixture", SecretKey: "fixture-only", Region: "us-east-1"}, func(context.Context) (net.Conn, error) {
		t.Error("unexpected upstream dial")
		return nil, errors.New("unexpected dial")
	})
	if err != nil {
		t.Fatal(err)
	}
	session := &webDataSession{database: "allowed", objectClient: client}
	page, err := session.executeS3(context.Background(), `{"operation":"list_buckets"}`)
	if err != nil || len(page.Rows) != 1 || page.Rows[0]["key"] != "allowed" {
		t.Fatalf("unexpected scoped page: %+v %v", page, err)
	}
	if _, err = session.executeS3(context.Background(), `{"operation":"list_objects","bucket":"other"}`); !errors.Is(err, objectaccess.ErrDenied) {
		t.Fatalf("scope bypass: %v", err)
	}
	session.objectClient = nil
	if _, err = session.executeS3(context.Background(), `{"operation":"list_buckets"}`); err == nil {
		t.Fatal("closed session accepted")
	}
}
