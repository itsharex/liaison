package objectaccess

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Fixture secrets are ephemeral, never copied from an environment or account.
func fixtureConfig(t *testing.T) Config {
	t.Helper()
	var secret [32]byte
	_, err := rand.Read(secret[:])
	require.NoError(t, err)
	return Config{Endpoint: "http://objects.invalid:9000", Region: "us-east-1", AccessKey: "fixture", SecretKey: hex.EncodeToString(secret[:]), Writable: true}
}

type noDeadlines struct{ net.Conn }

func (noDeadlines) SetDeadline(time.Time) error      { panic("connector deadline unsupported") }
func (noDeadlines) SetReadDeadline(time.Time) error  { panic("connector deadline unsupported") }
func (noDeadlines) SetWriteDeadline(time.Time) error { panic("connector deadline unsupported") }

func setup(t *testing.T, handler http.HandlerFunc) (*Client, *atomic.Int32) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	var calls atomic.Int32
	client, err := New(fixtureConfig(t), func(ctx context.Context) (net.Conn, error) {
		calls.Add(1)
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
		if err != nil {
			return nil, err
		}
		return noDeadlines{conn}, nil
	})
	require.NoError(t, err)
	return client, &calls
}

func TestClient_PreservesObjectKeysAndSignsRequests(t *testing.T) {
	for _, key := range []string{"a/../b", "a//b", "/leading", "中文/+%2F ?#", "dot/./key", "trailing/"} {
		t.Run(key, func(t *testing.T) {
			client, calls := setup(t, func(w http.ResponseWriter, r *http.Request) {
				if r.Close {
					t.Error("request must drain response before closing its tunnel")
				}
				if r.Host != "objects.invalid:9000" || r.URL.Path != "/demo-bucket/"+key {
					t.Errorf("object path changed: %s", r.URL.Path)
				}
				if !strings.Contains(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256 Credential=fixture/") || r.Header.Get("X-Amz-Date") == "" {
					t.Error("unsigned request")
				}
				switch r.Method {
				case http.MethodPut:
					body, err := io.ReadAll(r.Body)
					if err != nil || !bytes.Equal(body, []byte("payload")) {
						t.Error("upload body changed")
					}
					if r.Header.Get("If-None-Match") != "*" {
						t.Error("upload can overwrite")
					}
				case http.MethodGet:
					fmt.Fprint(w, "payload")
				case http.MethodDelete:
					w.WriteHeader(204)
				default:
					t.Error("unexpected method")
				}
			})
			require.NoError(t, client.Upload(context.Background(), "demo-bucket", key, []byte("payload")))
			var dst bytes.Buffer
			n, err := client.Download(context.Background(), "demo-bucket", key, &dst)
			require.NoError(t, err)
			require.EqualValues(t, 7, n)
			require.Equal(t, "payload", dst.String())
			require.NoError(t, client.Delete(context.Background(), "demo-bucket", key))
			require.EqualValues(t, 3, calls.Load(), "each request must obtain a fresh authorized stream")
		})
	}
}

func TestClient_ListPaginationAndEncodedNames(t *testing.T) {
	client, _ := setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if r.URL.Path == "/" {
			if r.URL.Query().Get("max-buckets") != "200" {
				t.Error("unbounded bucket page")
			}
			fmt.Fprint(w, `<ListAllMyBucketsResult><Buckets><Bucket><Name>demo-bucket</Name></Bucket></Buckets><ContinuationToken>next-bucket</ContinuationToken></ListAllMyBucketsResult>`)
			return
		}
		q := r.URL.Query()
		if q.Get("list-type") != "2" || q.Get("max-keys") != "200" || q.Get("prefix") != "a//" || q.Get("continuation-token") != "+/opaque==" || q.Get("encoding-type") != "url" {
			t.Error("list parameters changed")
		}
		fmt.Fprintf(w, "%s", `<ListBucketResult><Name>demo-bucket</Name><EncodingType>url</EncodingType><IsTruncated>true</IsTruncated><NextContinuationToken>next+/==</NextContinuationToken><CommonPrefixes><Prefix>a//%E4%B8%AD%E6%96%87/</Prefix></CommonPrefixes><Contents><Key>a//%252F+name</Key><Size>7</Size><ETag>etag</ETag></Contents></ListBucketResult>`)
	})
	buckets, err := client.ListBuckets(context.Background(), "")
	require.NoError(t, err)
	require.Equal(t, "next-bucket", buckets.NextToken)
	require.Equal(t, "demo-bucket", buckets.Items[0].Name)
	list, err := client.List(context.Background(), "demo-bucket", "a//", "+/opaque==")
	require.NoError(t, err)
	require.Equal(t, "next+/==", list.NextToken)
	require.Equal(t, []string{"a//中文/"}, list.Prefixes)
	require.Equal(t, "a//%2F+name", list.Items[0].Key, "decode percent encoding once and never convert + to space")
}

func TestClient_NoRedirectOrProviderSecretLeak(t *testing.T) {
	for _, status := range []int{301, 307, 403, 404, 412, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var escaped atomic.Int32
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
			defer other.Close()
			client, calls := setup(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Location", other.URL)
				w.WriteHeader(status)
				fmt.Fprint(w, `<Error><Code>Failure</Code><Message>private-upstream-detail</Message></Error>`)
			})
			_, err := client.ListBuckets(context.Background(), "")
			require.Error(t, err)
			require.NotContains(t, err.Error(), "private-upstream-detail")
			require.EqualValues(t, 0, escaped.Load())
			require.EqualValues(t, 1, calls.Load(), "no hidden retries")
			if status == 403 {
				require.ErrorIs(t, err, ErrDenied)
			}
			if status == 404 {
				require.ErrorIs(t, err, ErrNotFound)
			}
			if status == 412 {
				require.ErrorIs(t, err, ErrExists)
			}
		})
	}
}

func TestClient_ValidationAndReadOnlyNeverDial(t *testing.T) {
	var calls atomic.Int32
	config := fixtureConfig(t)
	config.Writable = false
	client, err := New(config, func(context.Context) (net.Conn, error) { calls.Add(1); return nil, errors.New("must not dial") })
	require.NoError(t, err)
	require.ErrorIs(t, client.Upload(context.Background(), "demo-bucket", "key", nil), ErrReadOnly)
	require.ErrorIs(t, client.Delete(context.Background(), "demo-bucket", "key"), ErrReadOnly)
	for _, bucket := range []string{"", "../bucket", "bucket/path", "bucket?x", "127.0.0.1", "arn:aws:s3:us-east-1:123:accesspoint/test"} {
		_, err = client.List(context.Background(), bucket, "", "")
		require.ErrorIs(t, err, ErrInvalid)
	}
	_, err = client.Download(context.Background(), "demo-bucket", strings.Repeat("a", 1025), io.Discard)
	require.ErrorIs(t, err, ErrInvalid)
	require.EqualValues(t, 0, calls.Load())
	for _, endpoint := range []string{"file:///tmp/data", "http://user:pass@example.test", "http://example.test/path", "http://example.test?token=x", "http://example.test#fragment"} {
		config.Endpoint = endpoint
		_, err = New(config, func(context.Context) (net.Conn, error) { return nil, nil })
		require.ErrorIs(t, err, ErrInvalid)
	}
}

func TestClient_CancellationClosesConnectorWithoutDeadlines(t *testing.T) {
	started := make(chan struct{})
	closed := make(chan struct{})
	client, _ := setup(t, func(w http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(closed) })
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := client.ListBuckets(ctx, ""); done <- err }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(3 * time.Second):
		t.Fatal("request stuck")
	}
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("stream leaked")
	}
}

func TestClient_BoundsResponsesAndRejectsMalformedXML(t *testing.T) {
	for _, body := range []string{"not xml", `<ListBucketResult><IsTruncated>true</IsTruncated></ListBucketResult>`, strings.Repeat("x", listingLimit+1)} {
		client, _ := setup(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, body) })
		_, err := client.List(context.Background(), "demo-bucket", "", "")
		require.Error(t, err)
	}
	client, _ := setup(t, func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, strings.Repeat("x", 3*1024*1024)) })
	n, err := client.Download(context.Background(), "demo-bucket", "key", io.Discard)
	require.NoError(t, err)
	require.EqualValues(t, 3*1024*1024, n, "object transfer is not limited to XML page size")
	client, _ = setup(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(TransferLimit+1))
		w.WriteHeader(200)
	})
	_, err = client.Download(context.Background(), "demo-bucket", "key", io.Discard)
	require.ErrorIs(t, err, ErrTooLarge)
}

func TestTransport_RejectsChangedAuthorityBeforeDial(t *testing.T) {
	u, err := url.Parse("https://objects.invalid:9000")
	require.NoError(t, err)
	var calls atomic.Int32
	transport := &pinnedTransport{endpoint: *u, dial: func(context.Context) (net.Conn, error) { calls.Add(1); return nil, ErrDenied }}
	for _, target := range []string{"https://other.invalid/bucket", "http://objects.invalid:9000/bucket", "https://objects.invalid/bucket"} {
		req, err := http.NewRequestWithContext(context.Background(), "GET", target, nil)
		require.NoError(t, err)
		_, err = transport.RoundTrip(req)
		require.ErrorIs(t, err, ErrInvalid)
	}
	req, err := http.NewRequestWithContext(context.Background(), "GET", u.String(), nil)
	require.NoError(t, err)
	req.Host = "other.invalid"
	_, err = transport.RoundTrip(req)
	require.ErrorIs(t, err, ErrInvalid)
	require.EqualValues(t, 0, calls.Load())
}

func TestClient_DoesNotTrustUnverifiedTLS(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unverified request reached application") }))
	defer server.Close()
	config := fixtureConfig(t)
	config.Endpoint = "https://objects.invalid:9000"
	client, err := New(config, func(ctx context.Context) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, "tcp", server.Listener.Addr().String())
	})
	require.NoError(t, err)
	_, err = client.ListBuckets(context.Background(), "")
	require.ErrorIs(t, err, ErrUnavailable)
}

func TestClient_BinaryObjectRoundTrip(t *testing.T) {
	payload := []byte{0, 0xff, '\r', '\n', 0x80, 'x'}
	client, _ := setup(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			body, err := io.ReadAll(r.Body)
			if err != nil || !bytes.Equal(payload, body) {
				t.Error("binary upload changed")
			}
			return
		}
		if _, err := w.Write(payload); err != nil {
			t.Error(err)
		}
	})
	require.NoError(t, client.Upload(context.Background(), "demo-bucket", "binary", payload))
	var out bytes.Buffer
	_, err := client.Download(context.Background(), "demo-bucket", "binary", &out)
	require.NoError(t, err)
	require.Equal(t, payload, out.Bytes())
}
