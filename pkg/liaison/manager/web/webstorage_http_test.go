package web

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/manager/controlplane"
	"github.com/liaisonio/liaison/pkg/liaison/manager/iam"
	"github.com/liaisonio/liaison/pkg/liaison/manager/objectaccess"
	"github.com/liaisonio/liaison/pkg/liaison/repo/model"
	"github.com/stretchr/testify/require"
)

type storageHTTPControl struct {
	controlplane.ControlPlane
	target *controlplane.WebDataTarget
	audits []*controlplane.WebDataAudit
}

func (c *storageHTTPControl) GetWebDataTarget(context.Context, uint) (*controlplane.WebDataTarget, error) {
	return c.target, nil
}
func (c *storageHTTPControl) RecordWebDataAudit(_ context.Context, a *controlplane.WebDataAudit) error {
	c.audits = append(c.audits, a)
	return nil
}

func TestStorageHTTPIsolationPermissionAndTransfers(t *testing.T) {
	w, admin, user := newPermissionHTTPTest(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(out http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method == http.MethodPut {
			require.Equal(t, "*", r.Header.Get("If-None-Match"))
			if r.URL.Path == "/allowed/exists" {
				out.WriteHeader(412)
				fmt.Fprint(out, `<Error><Code>PreconditionFailed</Code></Error>`)
				return
			}
			out.WriteHeader(200)
			return
		}
		fmt.Fprint(out, "payload")
	}))
	t.Cleanup(server.Close)
	client, err := objectaccess.New(objectaccess.Config{Endpoint: "http://storage.invalid", Region: "us-east-1", AccessKey: "fixture", SecretKey: "fixture-only", Writable: true}, func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	})
	require.NoError(t, err)
	cp := &storageHTTPControl{target: &controlplane.WebDataTarget{ProxyID: 1, ApplicationID: 2, Protocol: "s3", EffectiveStatus: "active"}}
	w.controlPlane = cp
	w.webData = newWebDataSessionStore()
	session, err := w.webData.create(&webDataSession{userID: user.ID, proxyID: 1, protocol: "s3", database: "allowed", objectClient: client, target: cp.target})
	require.NoError(t, err)
	call := func(actor *model.User, action, bucket, key, body string) *httptest.ResponseRecorder {
		method := http.MethodPost
		if action == "download" || action == "capabilities" {
			method = http.MethodGet
		}
		r := httptest.NewRequest(method, "/api/v1/webdata/sessions/"+session.token+"/storage/"+action+"?"+url.Values{"bucket": {bucket}, "key": {key}}.Encode(), strings.NewReader(body))
		if actor != nil {
			r = r.WithContext(context.WithValue(r.Context(), "user", actor))
		}
		out := httptest.NewRecorder()
		if action == "download" {
			w.handleWebStorageDownloadHTTP(out, r)
		} else {
			w.handleWebStorageHTTP(out, r)
		}
		return out
	}
	require.Equal(t, 401, call(nil, "download", "allowed", "key", "").Code)
	require.Equal(t, 401, call(admin, "download", "allowed", "key", "").Code)
	require.Equal(t, 403, call(user, "upload", "allowed", "key", "payload").Code)
	require.Equal(t, 0, calls)
	require.NoError(t, w.iamService.SetUserFeaturePolicy(admin, []string{iam.FeatureStorageUpload, iam.FeatureAccessAI}))
	require.Equal(t, 401, call(admin, "upload", "allowed", "key", "payload").Code)
	require.Equal(t, 403, call(user, "upload", "other", "key", "payload").Code)
	require.Equal(t, 0, calls)
	require.Equal(t, 200, call(user, "upload", "allowed", "key", "payload").Code)
	require.Equal(t, 409, call(user, "upload", "allowed", "exists", "payload").Code)
	require.Equal(t, 2, calls)
	require.Equal(t, 413, call(user, "upload", "allowed", "large", string(bytes.Repeat([]byte{'x'}, objectaccess.TransferLimit+1))).Code)
	require.Equal(t, 2, calls)
	download := call(user, "download", "allowed", "文件.txt", "")
	require.Equal(t, 200, download.Code)
	require.Equal(t, "payload", download.Body.String())
	require.Contains(t, download.Header().Get("Content-Disposition"), "attachment")
	require.Equal(t, "no-store", download.Header().Get("Cache-Control"))
	require.Equal(t, 403, call(user, "download", "other", "key", "").Code)
	require.Equal(t, 200, call(user, "context", "", "", `{"bucket":"allowed","prefix":"a//","key":"a//b"}`).Code)
	require.Equal(t, "a//b", session.storageContext.Key)
	handle := &webDataAgentHandle{web: w, session: session}
	schema, err := handle.Schema(context.Background(), nil)
	require.NoError(t, err)
	require.Contains(t, string(schema), `"bucket_scope":"allowed"`)
	require.Contains(t, string(schema), `"key":"a//b"`)
	require.Contains(t, string(schema), `"file_contents_available":false`)
	require.Contains(t, string(schema), "untrusted browser navigation")
	before := calls
	_, err = handle.Schema(context.Background(), []string{"bucket", "other"})
	require.ErrorIs(t, err, objectaccess.ErrDenied)
	for _, path := range [][]string{{"download", "allowed", "key"}, {"allowed", "allowed", "a//", ""}} {
		content, pathErr := handle.Schema(context.Background(), path)
		require.NoError(t, pathErr)
		require.Contains(t, string(content), `"error":`)
		require.Contains(t, string(content), `"retry_path":["bucket","allowed","a//",""]`)
	}
	require.Equal(t, before, calls, "invalid or cross-bucket tools must not reach upstream")
	for _, body := range []string{`{"bucket":"other"}`, `{"bucket":"allowed","prefix":"a/","key":"b"}`, `{"bucket":"allowed","secret":"bad"}`, `{"bucket":"allowed"} {}`} {
		require.Equal(t, 400, call(user, "context", "", "", body).Code)
	}
	require.NoError(t, w.iamService.SetUserFeaturePolicy(admin, []string{}))
	require.Equal(t, 403, call(user, "upload", "allowed", "key", "payload").Code)
	require.Equal(t, 403, call(user, "context", "", "", `{}`).Code)
	require.Contains(t, call(user, "capabilities", "", "", "").Body.String(), `"can_upload":false`)
	cp.target.EffectiveStatus = "inactive"
	require.Equal(t, 409, call(user, "download", "allowed", "key", "").Code)
	require.NotEmpty(t, cp.audits)
	for _, audit := range cp.audits {
		require.NotContains(t, audit.StatementPreview, "payload")
		require.NotContains(t, audit.StatementPreview, "fixture-only")
	}
}
