package web

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime"
	"net/http"
	"strings"
	"time"

	"github.com/liaisonio/liaison/pkg/liaison/manager/objectaccess"
)

// downloadS3 buffers the bounded object before returning it. Failed upstream
// transfers must not become successful HTTP attachments containing partial data.
// The caller holds session.mu, as for other WebData session operations.
func (session *webDataSession) downloadS3(ctx context.Context, bucket, key string) ([]byte, error) {
	if session.protocol != "s3" || session.objectClient == nil {
		return nil, objectaccess.ErrInvalid
	}
	if session.database != "" && session.database != bucket {
		return nil, objectaccess.ErrDenied
	}
	var dst bytes.Buffer
	if _, err := session.objectClient.Download(ctx, bucket, key, &dst); err != nil {
		return nil, err
	}
	return dst.Bytes(), nil
}

func storageDownloadFilename(key string) string {
	// Only the suggested local filename is sanitized, never the upstream key.
	name := key[strings.LastIndex(key, "/")+1:]
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || r == '\\' {
			return '_'
		}
		return r
	}, name)
	if name == "" || name == "." || name == ".." {
		return "download"
	}
	return name
}

func storageDownloadStatus(err error) int {
	switch {
	case errors.Is(err, objectaccess.ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, objectaccess.ErrDenied):
		return http.StatusForbidden
	case errors.Is(err, objectaccess.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, objectaccess.ErrTooLarge):
		return http.StatusRequestEntityTooLarge
	case errors.Is(err, context.DeadlineExceeded):
		return http.StatusGatewayTimeout
	default:
		return http.StatusBadGateway
	}
}

// handleWebStorageDownloadHTTP downloads a bounded S3 object through its session.
// @Summary Download an object from the current WebS3 session
// @Router /api/v1/webdata/sessions/{token}/storage/download [get]
// @Success 200 {file} binary
func (web *web) handleWebStorageDownloadHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"code": 405, "message": "method not allowed"})
		return
	}
	user, err := web.authenticateHTTP(r)
	if err != nil {
		writeUnauthorized(w)
		return
	}
	token, err := parseWebDataSessionToken(r, "/storage/download")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"code": 400, "message": "invalid webdata session"})
		return
	}
	session, ok := web.webData.get(token)
	if !ok || session.userID != user.ID {
		writeUnauthorized(w)
		return
	}
	if err := web.ensureWebDataSessionActive(r.Context(), session); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"code": 409, "message": "session is no longer available; reconnect", "reason": "SESSION_UNAVAILABLE"})
		return
	}
	bucket, key := r.URL.Query().Get("bucket"), r.URL.Query().Get("key")
	session.mu.Lock()
	defer session.mu.Unlock()
	ctx, cancel := context.WithTimeout(r.Context(), webDataExecuteTimeout)
	defer cancel()
	started := time.Now()
	data, err := session.downloadS3(ctx, bucket, key)
	message := ""
	if err != nil {
		message = "object download failed"
	}
	statement, marshalErr := json.Marshal(map[string]string{"operation": "download", "bucket": bucket, "key": key})
	if marshalErr != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"code": 500, "message": "object download failed"})
		return
	}
	web.recordWebDataAudit(r, session.target, user.ID, "download", "s3", bucket, string(statement), err == nil, 0, time.Since(started).Milliseconds(), message)
	if err != nil {
		status := storageDownloadStatus(err)
		writeJSON(w, status, map[string]any{"code": status, "message": message})
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": storageDownloadFilename(key)}))
	// ServeContent handles client disconnects without exposing provider responses.
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(data))
}
