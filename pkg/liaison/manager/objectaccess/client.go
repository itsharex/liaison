// Package objectaccess accesses S3-compatible services over caller-owned tunnels.
// The caller owns user authorization, credential storage, approval and auditing.
// No default credential chain, public presigned URLs or local network dial is used.
package objectaccess

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	TransferLimit = 16 * 1024 * 1024
	listingLimit  = 2 * 1024 * 1024
	PageLimit     = 200
)

var (
	ErrInvalid     = errors.New("invalid object storage request")
	ErrReadOnly    = errors.New("object storage session is read-only")
	ErrTooLarge    = errors.New("object storage response exceeds limit")
	ErrUnavailable = errors.New("object storage request failed")
	ErrExists      = errors.New("object already exists")
	ErrDenied      = errors.New("object storage permission denied")
	ErrNotFound    = errors.New("object not found")
	bucketPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]{1,61}[a-z0-9]$`)
	regionPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)
)

// Dial must reauthorize the session and return a fresh stream to its registered
// application on every invocation. There is deliberately no address argument.
type Dial func(context.Context) (net.Conn, error)

type Config struct {
	// Endpoint is constructed from trusted application metadata, not a browser URL.
	Endpoint     string
	Region       string
	AccessKey    string `json:"-"`
	SecretKey    string `json:"-"`
	SessionToken string `json:"-"`
	Writable     bool
}

type Client struct {
	sdk      *s3.Client
	writable bool
}

// New configures path-style S3 only. TLS uses the registered host and system roots.
func New(config Config, dial Dial) (*Client, error) {
	u, err := url.Parse(config.Endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "http" && u.Scheme != "https") || dial == nil {
		return nil, ErrInvalid
	}
	if !regionPattern.MatchString(config.Region) || config.AccessKey == "" || config.SecretKey == "" || len(config.AccessKey) > 256 || len(config.SecretKey) > 4096 || len(config.SessionToken) > 8192 || strings.ContainsAny(config.AccessKey+config.SecretKey+config.SessionToken, "\r\n\x00") {
		return nil, ErrInvalid
	}
	u.Path = ""
	credentials := aws.Credentials{AccessKeyID: config.AccessKey, SecretAccessKey: config.SecretKey, SessionToken: config.SessionToken}
	transport := &pinnedTransport{endpoint: *u, dial: dial}
	sdk := s3.New(s3.Options{
		BaseEndpoint: aws.String(u.String()), Region: config.Region, UsePathStyle: true,
		Credentials:                aws.CredentialsProviderFunc(func(context.Context) (aws.Credentials, error) { return credentials, nil }),
		HTTPClient:                 &http.Client{Transport: transport, Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		RetryMaxAttempts:           1,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired,
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})
	return &Client{sdk: sdk, writable: config.Writable}, nil
}

type pinnedTransport struct {
	endpoint url.URL
	dial     Dial
}

func (p *pinnedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != p.endpoint.Scheme || req.URL.Host != p.endpoint.Host || (req.Host != "" && req.Host != p.endpoint.Host) || req.URL.User != nil {
		return nil, ErrInvalid
	}
	// Each request owns its transport, so connections cannot be reused across
	// authorization checks. Do not send Connection: close: a tunnel peer may
	// close its stream on upstream EOF before the final response is drained.
	// Close this private transport only after the SDK has consumed the body.
	t := &http.Transport{DisableCompression: true, MaxResponseHeaderBytes: 32 * 1024, ResponseHeaderTimeout: 20 * time.Second}
	t.DialContext = func(_ context.Context, _, _ string) (net.Conn, error) { return p.dial(req.Context()) }
	t.DialTLSContext = func(_ context.Context, _, _ string) (net.Conn, error) {
		conn, err := p.dial(req.Context())
		if err != nil {
			return nil, err
		}
		secured := tls.Client(conn, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: p.endpoint.Hostname()})
		if err = secured.HandshakeContext(req.Context()); err != nil {
			conn.Close()
			return nil, err
		}
		return secured, nil
	}
	resp, err := t.RoundTrip(req)
	if err != nil {
		t.CloseIdleConnections()
		return nil, err
	}
	limit := int64(listingLimit)
	if req.Method == http.MethodGet && !req.URL.Query().Has("list-type") && strings.Count(req.URL.Path, "/") >= 2 {
		limit = TransferLimit
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		limit = 64 * 1024
	}
	if resp.ContentLength > limit {
		resp.Body.Close()
		t.CloseIdleConnections()
		return nil, ErrTooLarge
	}
	resp.Body = &boundedBody{ReadCloser: resp.Body, remaining: limit, afterClose: t.CloseIdleConnections}
	return resp, nil
}

type boundedBody struct {
	io.ReadCloser
	remaining  int64
	afterClose func()
}

func (b *boundedBody) Close() error {
	err := b.ReadCloser.Close()
	if b.afterClose != nil {
		b.afterClose()
	}
	return err
}

func (b *boundedBody) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if b.remaining == 0 {
		var probe [1]byte
		n, err := b.ReadCloser.Read(probe[:])
		if n > 0 {
			return 0, ErrTooLarge
		}
		return 0, err
	}
	if int64(len(p)) > b.remaining {
		p = p[:b.remaining]
	}
	n, err := b.ReadCloser.Read(p)
	b.remaining -= int64(n)
	return n, err
}

func validBucket(bucket string) bool {
	return bucketPattern.MatchString(bucket) && !strings.Contains(bucket, "..") && net.ParseIP(bucket) == nil
}
func validKey(key string, empty bool) bool {
	return (empty || key != "") && len(key) <= 1024 && utf8.ValidString(key)
}

// safeError deliberately discards provider messages, request URLs and credentials.
func safeError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if errors.Is(err, ErrTooLarge) {
		return ErrTooLarge
	}
	var status interface{ HTTPStatusCode() int }
	if errors.As(err, &status) {
		switch status.HTTPStatusCode() {
		case 403:
			return ErrDenied
		case 404:
			return ErrNotFound
		case 412:
			return ErrExists
		}
	}
	return ErrUnavailable
}

type Bucket struct {
	Name string `json:"name"`
}
type Buckets struct {
	Items     []Bucket `json:"items"`
	NextToken string   `json:"next_token,omitempty"`
}

func (c *Client) ListBuckets(ctx context.Context, token string) (Buckets, error) {
	if len(token) > 8192 {
		return Buckets{}, ErrInvalid
	}
	in := &s3.ListBucketsInput{MaxBuckets: aws.Int32(PageLimit)}
	if token != "" {
		in.ContinuationToken = aws.String(token)
	}
	out, err := c.sdk.ListBuckets(ctx, in)
	if err != nil {
		return Buckets{}, safeError(ctx, err)
	}
	if len(out.Buckets) > PageLimit {
		return Buckets{}, ErrTooLarge
	}
	result := Buckets{Items: make([]Bucket, 0, len(out.Buckets)), NextToken: aws.ToString(out.ContinuationToken)}
	for _, bucket := range out.Buckets {
		result.Items = append(result.Items, Bucket{Name: aws.ToString(bucket.Name)})
	}
	return result, nil
}

type Object struct {
	Key        string     `json:"key"`
	Size       int64      `json:"size"`
	ModifiedAt *time.Time `json:"modified_at,omitempty"`
	ETag       string     `json:"etag"`
}
type Objects struct {
	Items     []Object `json:"items"`
	Prefixes  []string `json:"prefixes"`
	NextToken string   `json:"next_token,omitempty"`
}

func (c *Client) List(ctx context.Context, bucket, prefix, token string) (Objects, error) {
	if !validBucket(bucket) || !validKey(prefix, true) || len(token) > 8192 {
		return Objects{}, ErrInvalid
	}
	in := &s3.ListObjectsV2Input{Bucket: aws.String(bucket), Prefix: aws.String(prefix), Delimiter: aws.String("/"), MaxKeys: aws.Int32(PageLimit), EncodingType: types.EncodingTypeUrl}
	if token != "" {
		in.ContinuationToken = aws.String(token)
	}
	out, err := c.sdk.ListObjectsV2(ctx, in)
	if err != nil {
		return Objects{}, safeError(ctx, err)
	}
	if aws.ToString(out.Name) != bucket {
		return Objects{}, ErrUnavailable
	}
	if len(out.Contents)+len(out.CommonPrefixes) > PageLimit {
		return Objects{}, ErrTooLarge
	}
	result := Objects{Items: []Object{}, Prefixes: []string{}, NextToken: aws.ToString(out.NextContinuationToken)}
	decode := func(value string) (string, error) {
		if out.EncodingType == types.EncodingTypeUrl {
			return url.PathUnescape(value)
		}
		return value, nil
	}
	for _, item := range out.Contents {
		key, err := decode(aws.ToString(item.Key))
		if err != nil {
			return Objects{}, ErrUnavailable
		}
		result.Items = append(result.Items, Object{Key: key, Size: aws.ToInt64(item.Size), ModifiedAt: item.LastModified, ETag: aws.ToString(item.ETag)})
	}
	for _, item := range out.CommonPrefixes {
		key, err := decode(aws.ToString(item.Prefix))
		if err != nil {
			return Objects{}, ErrUnavailable
		}
		result.Prefixes = append(result.Prefixes, key)
	}
	if aws.ToBool(out.IsTruncated) && result.NextToken == "" {
		return Objects{}, ErrUnavailable
	}
	return result, nil
}

// Download never emits a direct or presigned upstream URL. dst must unblock when
// ctx ends (the HTTP handler owns its response write deadline).
func (c *Client) Download(ctx context.Context, bucket, key string, dst io.Writer) (int64, error) {
	if !validBucket(bucket) || !validKey(key, false) || dst == nil {
		return 0, ErrInvalid
	}
	out, err := c.sdk.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return 0, safeError(ctx, err)
	}
	defer out.Body.Close() // release the dedicated stream on success or failure
	n, err := io.Copy(dst, out.Body)
	return n, safeError(ctx, err)
}

// Upload is deliberately bounded and create-only. No overwrite or bucket-level
// destructive operations are exposed in the initial adapter.
func (c *Client) Upload(ctx context.Context, bucket, key string, data []byte) error {
	if !c.writable {
		return ErrReadOnly
	}
	if !validBucket(bucket) || !validKey(key, false) {
		return ErrInvalid
	}
	if len(data) > TransferLimit {
		return ErrTooLarge
	}
	_, err := c.sdk.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(data), IfNoneMatch: aws.String("*"), ContentType: aws.String("application/octet-stream")})
	return safeError(ctx, err)
}

func (c *Client) Delete(ctx context.Context, bucket, key string) error {
	if !c.writable {
		return ErrReadOnly
	}
	if !validBucket(bucket) || !validKey(key, false) {
		return ErrInvalid
	}
	_, err := c.sdk.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	return safeError(ctx, err)
}
