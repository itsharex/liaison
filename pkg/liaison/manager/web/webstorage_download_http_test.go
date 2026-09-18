package web

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/manager/objectaccess"
	"github.com/stretchr/testify/require"
)

func TestStorageDownloadScopeAndClosedSession(t *testing.T) {
	client, err := objectaccess.New(objectaccess.Config{Endpoint: "http://objects.invalid", Region: "us-east-1", AccessKey: "fixture", SecretKey: "fixture-only"}, func(context.Context) (net.Conn, error) {
		t.Error("unauthorized request dialed upstream")
		return nil, errors.New("unexpected dial")
	})
	require.NoError(t, err)
	session := &webDataSession{protocol: "s3", database: "allowed", objectClient: client}
	data, err := session.downloadS3(context.Background(), "other", "key")
	require.ErrorIs(t, err, objectaccess.ErrDenied)
	require.Nil(t, data)
	session.protocol = "mysql"
	_, err = session.downloadS3(context.Background(), "allowed", "key")
	require.ErrorIs(t, err, objectaccess.ErrInvalid)
	session.protocol, session.objectClient = "s3", nil
	_, err = session.downloadS3(context.Background(), "allowed", "key")
	require.ErrorIs(t, err, objectaccess.ErrInvalid)
}

func TestStorageDownloadPreservesKeysAndDiscardsPartialData(t *testing.T) {
	for _, truncated := range []bool{false, true} {
		t.Run(fmt.Sprint(truncated), func(t *testing.T) {
			key := "folder//../ 文件+.txt "
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, "/allowed/"+key, r.URL.Path)
				if truncated {
					w.Header().Set("Content-Length", "100")
				}
				fmt.Fprint(w, "payload")
			}))
			t.Cleanup(server.Close)
			client, err := objectaccess.New(objectaccess.Config{Endpoint: "http://objects.invalid", Region: "us-east-1", AccessKey: "fixture", SecretKey: "fixture-only"}, func(ctx context.Context) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
			})
			require.NoError(t, err)
			session := &webDataSession{protocol: "s3", database: "allowed", objectClient: client}
			data, err := session.downloadS3(context.Background(), "allowed", key)
			if truncated {
				require.Error(t, err)
				require.Nil(t, data)
			} else {
				require.NoError(t, err)
				require.Equal(t, "payload", string(data))
			}
		})
	}
}

func TestStorageDownloadFilename(t *testing.T) {
	for key, want := range map[string]string{"dir/file.txt": "file.txt", "dir/": "download", "..": "download", "dir/a\r\nb\\c": "a__b_c", "目录/文件.txt": "文件.txt"} {
		require.Equal(t, want, storageDownloadFilename(key))
	}
}

func TestStorageDownloadStatus(t *testing.T) {
	for err, status := range map[error]int{objectaccess.ErrInvalid: 400, objectaccess.ErrDenied: 403, objectaccess.ErrNotFound: 404, objectaccess.ErrTooLarge: 413, context.DeadlineExceeded: 504, objectaccess.ErrUnavailable: 502} {
		require.Equal(t, status, storageDownloadStatus(fmt.Errorf("wrapped: %w", err)))
	}
}

func TestStorageDownloadRejectsMutationBeforeAuthentication(t *testing.T) {
	web := &web{}
	w := httptest.NewRecorder()
	web.handleWebStorageDownloadHTTP(w, httptest.NewRequest(http.MethodPost, "/api/v1/webdata/sessions/test/storage/download", nil))
	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	require.Equal(t, "GET", w.Header().Get("Allow"))
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}
