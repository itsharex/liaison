package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"

	"github.com/jumboframes/armorigo/log"
	"github.com/liaisonio/liaison/pkg/liaison/manager/objectaccess"
)

type storageCommand struct {
	Operation string `json:"operation"`
	Bucket    string `json:"bucket"`
	Prefix    string `json:"prefix"`
	Token     string `json:"continuation_token"`
}

func parseStorageCommand(statement string) (storageCommand, error) {
	var command storageCommand
	if len(statement) > 16*1024 {
		return command, objectaccess.ErrInvalid
	}
	decoder := json.NewDecoder(strings.NewReader(statement))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&command) != nil || decoder.Decode(new(any)) != io.EOF {
		return command, objectaccess.ErrInvalid
	}
	if command.Operation != "list_buckets" && command.Operation != "list_objects" {
		return command, objectaccess.ErrReadOnly
	}
	if command.Operation == "list_buckets" && (command.Bucket != "" || command.Prefix != "") {
		return command, objectaccess.ErrInvalid
	}
	return command, nil
}

func (web *web) openWebDataS3(ctx context.Context, session *webDataSession, secret string) error {
	if session.connectionParams != "" || session.authMechanism != "" || (session.authDatabase != "" && session.authDatabase != "admin") {
		return objectaccess.ErrInvalid
	}
	scheme := "https"
	switch session.tlsMode {
	case "require", "":
	case "disable":
		scheme = "http"
	default:
		return objectaccess.ErrInvalid
	}
	region := session.schema
	if region == "" {
		region = "us-east-1"
	}
	target := session.target
	endpoint := scheme + "://" + net.JoinHostPort(target.TargetHost, strconv.Itoa(target.TargetPort))
	client, err := objectaccess.New(objectaccess.Config{Endpoint: endpoint, Region: region, AccessKey: session.username, SecretKey: secret, Writable: false}, func(ctx context.Context) (net.Conn, error) {
		// HTTP execution has already checked session ownership. Reauthorize each
		// fresh stream as that owner, not an absent request-context identity.
		ctx = context.WithValue(ctx, "user_id", session.userID)
		conn, current, err := web.controlPlane.OpenWebDataStream(ctx, session.proxyID)
		if err != nil {
			return nil, err
		}
		if current.Protocol != "s3" || current.ApplicationID != target.ApplicationID || current.TargetHost != target.TargetHost || current.TargetPort != target.TargetPort {
			conn.Close()
			return nil, errors.New("object storage target changed; reconnect")
		}
		return conn, nil
	})
	if err != nil {
		return err
	}
	// A bucket-scoped credential need not have ListAllMyBuckets permission.
	if session.database != "" {
		_, err = client.List(ctx, session.database, "", "")
	} else {
		_, err = client.ListBuckets(ctx, "")
	}
	if err != nil {
		// objectaccess errors are redacted; never log raw provider responses.
		log.Warnf("webs3 connection failed: proxy_id=%d error=%v", session.proxyID, err)
		return err
	}
	session.objectClient = client
	return nil
}

func (session *webDataSession) executeS3(ctx context.Context, statement string) (*webDataExecuteResponse, error) {
	command, err := parseStorageCommand(statement)
	if err != nil {
		return nil, err
	}
	if session.objectClient == nil {
		return nil, errors.New("object storage session closed")
	}
	result := &webDataExecuteResponse{Type: "rows", Columns: []string{"kind", "key", "size", "modified_at", "etag"}, Rows: []map[string]any{}}
	if command.Operation == "list_buckets" {
		if session.database != "" {
			result.Rows = append(result.Rows, map[string]any{"kind": "bucket", "key": session.database})
			return result, nil
		}
		page, err := session.objectClient.ListBuckets(ctx, command.Token)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			result.Rows = append(result.Rows, map[string]any{"kind": "bucket", "key": item.Name})
		}
		result.NextToken = page.NextToken
		return result, nil
	}
	if session.database != "" && command.Bucket != session.database {
		return nil, objectaccess.ErrDenied
	}
	page, err := session.objectClient.List(ctx, command.Bucket, command.Prefix, command.Token)
	if err != nil {
		return nil, err
	}
	for _, prefix := range page.Prefixes {
		result.Rows = append(result.Rows, map[string]any{"kind": "prefix", "key": prefix})
	}
	for _, item := range page.Items {
		result.Rows = append(result.Rows, map[string]any{"kind": "object", "key": item.Key, "size": item.Size, "modified_at": item.ModifiedAt, "etag": item.ETag})
	}
	result.NextToken = page.NextToken
	return result, nil
}
