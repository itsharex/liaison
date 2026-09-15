package web

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/liaisonio/liaison/pkg/liaison/manager/iam"
	"github.com/liaisonio/liaison/pkg/liaison/manager/objectaccess"
)

type storageWorkspaceContext struct {
	Bucket string `json:"bucket"`
	Prefix string `json:"prefix"`
	Key    string `json:"key"`
}

func (s *webDataSession) storageListingPath() []string {
	if s.storageContext.Bucket == "" {
		return []string{}
	}
	return []string{"bucket", s.storageContext.Bucket, s.storageContext.Prefix, ""}
}

func (s *webDataSession) validateStorageContext(v storageWorkspaceContext) error {
	if s.protocol != "s3" || s.objectClient == nil {
		return objectaccess.ErrInvalid
	}
	if s.database != "" && v.Bucket != "" && s.database != v.Bucket {
		return objectaccess.ErrDenied
	}
	if len(v.Bucket) > 63 || len(v.Prefix) > 1024 || len(v.Key) > 1024 || !utf8.ValidString(v.Bucket+v.Prefix+v.Key) || strings.ContainsAny(v.Bucket+v.Prefix+v.Key, "\x00\r\n") {
		return objectaccess.ErrInvalid
	}
	if v.Bucket == "" && (v.Prefix != "" || v.Key != "") || v.Key != "" && !strings.HasPrefix(v.Key, v.Prefix) {
		return objectaccess.ErrInvalid
	}
	return nil
}

// storageSchema uses the existing read-only schema tool, not the query executor.
// The caller holds session.mu. Browser context is never used as authorization.
func (h *webDataAgentHandle) storageSchema(ctx context.Context, path []string) (json.RawMessage, error) {
	command := storageCommand{Operation: "list_buckets"}
	if len(path) == 2 && path[0] == "buckets" {
		command.Token = path[1]
	} else if len(path) > 0 {
		if len(path) < 2 || len(path) > 4 || path[0] != "bucket" {
			return nil, objectaccess.ErrInvalid
		}
		command.Operation, command.Bucket = "list_objects", path[1]
		if len(path) > 2 {
			command.Prefix = path[2]
		}
		if len(path) > 3 {
			command.Token = path[3]
		}
	}
	statement, err := json.Marshal(command)
	if err != nil {
		return nil, err
	}
	started := time.Now()
	result, err := h.session.executeS3(ctx, string(statement))
	if result == nil {
		result = &webDataExecuteResponse{}
	}
	result.ElapsedMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = err.Error()
	}
	h.auditQuery(ctx, string(statement), result, err)
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]any{"protocol": "s3", "browser_context": h.session.storageContext, "context_source": "untrusted browser navigation; not object existence or file contents", "bucket_scope": h.session.database, "page": result, "file_contents_available": false, "current_listing_path": h.session.storageListingPath(), "pagination": "For more buckets use path=[\"buckets\",next_token]; for objects use path=[\"bucket\",bucket_name,prefix,next_token]. The first element is a fixed literal. Stop when next_token is empty."})
}

// handleWebStorageHTTP handles bounded storage uploads and navigation context.
// @Summary WebS3 upload, capabilities and Agent navigation context
// @Router /api/v1/webdata/sessions/{token}/storage/upload [post]
// @Success 200 {object} map[string]interface{}
func (web *web) handleWebStorageHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	action := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
	method := http.MethodPost
	if action == "capabilities" {
		method = http.MethodGet
	}
	if action != "upload" && action != "context" && action != "capabilities" {
		http.NotFound(w, r)
		return
	}
	if r.Method != method {
		w.Header().Set("Allow", method)
		writeJSON(w, 405, map[string]any{"code": 405, "message": "method not allowed"})
		return
	}
	actor, err := web.authenticateHTTP(r)
	if err != nil {
		writeUnauthorized(w)
		return
	}
	feature := ""
	if action == "upload" {
		feature = iam.FeatureStorageUpload
	}
	if action == "context" {
		feature = iam.FeatureAccessAI
	}
	if feature != "" && web.iamService.RequireFeature(actor, feature) != nil {
		writeJSON(w, 403, map[string]any{"code": 403, "message": "permission denied"})
		return
	}
	token, err := parseWebDataSessionToken(r, "/storage/"+action)
	if err != nil {
		writeJSON(w, 400, map[string]any{"code": 400, "message": "invalid session"})
		return
	}
	session, ok := web.webData.get(token)
	if !ok || session.userID != actor.ID {
		writeUnauthorized(w)
		return
	}
	if err = web.ensureWebDataSessionActive(r.Context(), session); err != nil {
		writeJSON(w, 409, map[string]any{"code": 409, "message": "session unavailable", "reason": "SESSION_UNAVAILABLE"})
		return
	}
	if action != "capabilities" {
		// Bound slow request bodies as well as their size, including while waiting
		// for the session lock. HTTP/1 and HTTP/2 implement this controller hook.
		controller := http.NewResponseController(w)
		if controller.SetReadDeadline(time.Now().Add(30*time.Second)) == nil {
			defer controller.SetReadDeadline(time.Time{})
		}
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.protocol != "s3" || session.objectClient == nil {
		writeJSON(w, 409, map[string]any{"code": 409, "message": "session unavailable", "reason": "SESSION_UNAVAILABLE"})
		return
	}
	if action == "capabilities" {
		writeJSON(w, 200, map[string]any{"code": 200, "data": map[string]any{"can_upload": web.iamService.RequireFeature(actor, iam.FeatureStorageUpload) == nil, "transfer_limit": objectaccess.TransferLimit}})
		return
	}
	if action == "context" {
		var value storageWorkspaceContext
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
		decoder.DisallowUnknownFields()
		if err = decoder.Decode(&value); err == nil && decoder.Decode(new(any)) != io.EOF {
			err = objectaccess.ErrInvalid
		}
		if err == nil {
			err = session.validateStorageContext(value)
		}
		if err != nil {
			writeJSON(w, 400, map[string]any{"code": 400, "message": "invalid storage context"})
			return
		}
		session.storageContext = value
		writeJSON(w, 200, map[string]any{"code": 200, "message": "success"})
		return
	}
	bucket, key := r.URL.Query().Get("bucket"), r.URL.Query().Get("key")
	started := time.Now()
	err = session.validateStorageContext(storageWorkspaceContext{Bucket: bucket, Key: key})
	if bucket == "" || key == "" {
		err = objectaccess.ErrInvalid
	}
	if err == nil {
		var data []byte
		data, err = io.ReadAll(http.MaxBytesReader(w, r.Body, objectaccess.TransferLimit))
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			err = objectaccess.ErrTooLarge
		}
		if err == nil {
			err = session.objectClient.Upload(r.Context(), bucket, key, data)
		}
	}
	statement, marshalErr := json.Marshal(map[string]string{"operation": "upload", "bucket": bucket, "key": key})
	if marshalErr != nil {
		writeJSON(w, 400, map[string]any{"code": 400, "message": "invalid upload"})
		return
	}
	message := ""
	if err != nil {
		message = "object upload failed"
	}
	web.recordWebDataAudit(r, session.target, actor.ID, "upload", "s3", bucket, string(statement), err == nil, 0, time.Since(started).Milliseconds(), message)
	if err != nil {
		status := storageDownloadStatus(err)
		if errors.Is(err, objectaccess.ErrExists) {
			status = 409
		}
		writeJSON(w, status, map[string]any{"code": status, "message": message})
		return
	}
	writeJSON(w, 200, map[string]any{"code": 200, "message": "success"})
}
