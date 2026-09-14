package web

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/liaisonio/liaison/pkg/liaison/manager/controlplane"
	"github.com/liaisonio/liaison/pkg/liaison/manager/iam"
	"github.com/liaisonio/liaison/pkg/liaison/manager/sftpfiles"
	"github.com/liaisonio/liaison/pkg/liaison/repo/model"
	"golang.org/x/crypto/ssh"
)

// fileActor deliberately revalidates the Bearer token, even if middleware put a
// cached actor in the context. Long-running transfers also use this check.
func (web *web) fileActor(r *http.Request) (*model.User, error) {
	token, ok := bearerToken(r)
	if !ok {
		return nil, errUnauthorized
	}
	if strings.HasPrefix(token, model.PATPlaintextPrefix) {
		return web.iamService.GetUserByPAT(token, iam.ExtractClientIP(r))
	}
	return web.iamService.GetUserByToken(token)
}
func (web *web) authorizeFiles(r *http.Request, proxy uint, upload bool) (*model.User, *controlplane.WebSSHTarget, error) {
	actor, err := web.fileActor(r)
	if err != nil {
		return nil, nil, errUnauthorized
	}
	if err = web.iamService.RequireFeature(actor, iam.FeatureFilesRead); err != nil {
		return nil, nil, iam.ErrForbidden
	}
	if upload {
		if err = web.iamService.RequireFeature(actor, iam.FeatureFilesUpload); err != nil {
			return nil, nil, iam.ErrForbidden
		}
	}
	ctx := context.WithValue(r.Context(), "user_id", actor.ID)
	target, err := web.controlPlane.GetWebSSHTarget(ctx, proxy)
	if err != nil {
		return nil, nil, errFileSessionMissing
	}
	if target.EffectiveStatus != "active" {
		return nil, nil, iam.ErrForbidden
	}
	return actor, target, nil
}
func (web *web) openFiles(ctx context.Context, v fileSession, writable bool) (*sftpfiles.Files, error) {
	ctx = context.WithValue(ctx, "user_id", v.userID)
	raw, target, err := web.controlPlane.OpenWebSSHStream(ctx, v.proxyID)
	if err != nil {
		return nil, err
	}
	// Connector streams do not safely support repeated SetDeadline calls.
	// Bound negotiation by closing only this independent stream on timeout.
	handshakeCtx, handshakeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer handshakeCancel()
	stop := context.AfterFunc(handshakeCtx, func() { _ = raw.Close() /* abort remote IO */ })
	password, err := web.decryptWebSSHPassword(v.encrypted, v.nonce)
	if err != nil {
		stop()
		_ = raw.Close()
		return nil, err
	}
	defer func() {
		for i := range password {
			password[i] = 0
		}
	}()
	conn, chans, requests, err := ssh.NewClientConn(raw, net.JoinHostPort(target.TargetHost, strconv.Itoa(target.TargetPort)), &ssh.ClientConfig{
		User: v.username, Auth: []ssh.AuthMethod{ssh.Password(string(password))}, HostKeyCallback: web.webSSHHostKeyCallback(ctx, v.proxyID), Timeout: 10 * time.Second,
	})
	if err != nil {
		stop()
		_ = raw.Close()
		return nil, err
	}
	files, err := sftpfiles.New(handshakeCtx, ssh.NewClient(conn, chans, requests), writable)
	stopped := stop()
	if err != nil {
		return nil, err
	}
	if !stopped || handshakeCtx.Err() != nil {
		_ = files.Close()
		return nil, handshakeCtx.Err()
	}
	return files, nil
}

// handleCreateFileSessionHTTP creates a user-scoped file session.
// @Summary Open WebSSH file session
// @Router /api/v1/webssh/proxies/{id}/files/sessions [post]
// @Success 200 {object} map[string]interface{}
func (web *web) handleCreateFileSessionHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		fileError(w, errors.New("method"), 405)
		return
	}
	id, err := strconv.ParseUint(mux.Vars(r)["id"], 10, 32)
	if err != nil || id == 0 {
		fileError(w, sftpfiles.ErrInvalidPath, 0)
		return
	}
	actor, target, err := web.authorizeFiles(r, uint(id), false)
	if err != nil {
		fileError(w, err, 0)
		return
	}
	var req createWebSSHSessionRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&req); err != nil {
		fileError(w, sftpfiles.ErrInvalidPath, 0)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err = validateWebSSHSessionCredentials(req); err != nil {
		fileError(w, sftpfiles.ErrInvalidPath, 0)
		return
	}
	token, _ := bearerToken(r)
	sessionID, err := randomWebSSHToken()
	if err != nil {
		fileError(w, err, 0)
		return
	}
	v := fileSession{id: sessionID, userID: actor.ID, proxyID: uint(id), username: req.Username, auth: fileAuth(token), expires: time.Now().Add(30 * time.Minute)}
	if req.Password != "" {
		v.encrypted, v.nonce, err = web.encryptWebSSHPassword([]byte(req.Password))
		req.Password = ""
	} else {
		secret, e := web.controlPlane.GetWebSSHCredentialSecret(context.WithValue(r.Context(), "user_id", actor.ID), uint(id), req.Username)
		err = e
		if e == nil {
			v.encrypted, v.nonce = secret.EncryptedPassword, secret.Nonce
		}
	}
	if err != nil {
		fileError(w, errors.New("credential unavailable"), 0)
		return
	}
	if err = web.files.add(v); err != nil {
		fileError(w, err, 0)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	_, release, err := web.files.acquire(v.id, v.userID, v.auth, cancel)
	if err != nil {
		web.files.remove(v.id, v.userID, v.auth)
		fileError(w, err, 0)
		return
	}
	defer release()
	files, err := web.openFiles(ctx, v, web.iamService.RequireFeature(actor, iam.FeatureFilesUpload) == nil)
	var home string
	if err == nil {
		defer files.Close()
		home, err = files.Home(ctx)
	}
	if err == nil && req.SaveCredential {
		credentialCtx := context.WithValue(ctx, "user_id", actor.ID)
		err = web.controlPlane.SaveWebSSHCredential(credentialCtx, v.proxyID, v.username, v.encrypted, v.nonce)
	}
	web.auditFiles(r, target, v, "sftp_open", "", 0, err)
	if err != nil {
		web.files.remove(v.id, v.userID, v.auth)
		fileError(w, err, 0)
		return
	}
	writeJSON(w, 200, map[string]any{"code": 200, "message": "success", "data": map[string]any{"id": v.id, "home": home, "can_upload": files.CanUpload(), "expires_at": v.expires}})
}

// handleFileOperationHTTP lists/previews/downloads/uploads remote files or closes a session.
// @Summary Operate on WebSSH files
// @Router /api/v1/webssh/files/sessions/{session}/{operation} [get]
// @Router /api/v1/webssh/files/sessions/{session}/upload [put]
// @Router /api/v1/webssh/files/sessions/{session} [delete]
// @Success 200 {object} map[string]interface{}
func (web *web) handleFileOperationHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	actor, err := web.fileActor(r)
	if err != nil {
		writeUnauthorized(w)
		return
	}
	token, _ := bearerToken(r)
	id := mux.Vars(r)["session"]
	op := mux.Vars(r)["operation"]
	if r.Method == http.MethodDelete && op == "" {
		if !web.files.remove(id, actor.ID, fileAuth(token)) {
			fileError(w, errFileSessionMissing, 0)
			return
		}
		writeJSON(w, 200, map[string]any{"code": 200, "message": "success"})
		return
	}
	if !(r.Method == http.MethodGet && (op == "list" || op == "preview" || op == "download") || r.Method == http.MethodPut && op == "upload") {
		fileError(w, sftpfiles.ErrInvalidPath, 405)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	v, release, err := web.files.acquire(id, actor.ID, fileAuth(token), cancel)
	if err != nil {
		fileError(w, err, 0)
		return
	}
	defer release()
	actor, target, err := web.authorizeFiles(r, v.proxyID, op == "upload")
	if err != nil {
		fileError(w, err, 0)
		return
	}
	// Close request IO as well as SSH when canceled; an upload body may be stalled.
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(5 * time.Minute))
	_ = controller.SetWriteDeadline(time.Now().Add(5 * time.Minute))
	bodyDone := make(chan struct{})
	stopBody := context.AfterFunc(ctx, func() {
		defer close(bodyDone)
		_ = controller.SetReadDeadline(time.Now())
		_ = controller.SetWriteDeadline(time.Now())
		_ = r.Body.Close()
	})
	checkDone := make(chan struct{})
	go func() {
		defer close(checkDone)
		defer func() {
			if recover() != nil {
				cancel()
			}
		}()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, _, e := web.authorizeFiles(r.WithContext(ctx), v.proxyID, op == "upload"); e != nil {
					cancel()
					return
				}
			}
		}
	}()
	defer func() {
		if !stopBody() {
			<-bodyDone
		}
		cancel()
		<-checkDone
		_ = controller.SetReadDeadline(time.Time{})
		_ = controller.SetWriteDeadline(time.Time{})
	}()
	unregister := web.webSSH.registerActive(v.proxyID, "sftp_"+v.id, cancel)
	defer unregister()
	files, err := web.openFiles(ctx, v, web.iamService.RequireFeature(actor, iam.FeatureFilesUpload) == nil)
	if err != nil {
		web.auditFiles(r, target, v, "sftp_"+op, "", 0, err)
		fileError(w, err, 0)
		return
	}
	defer files.Close()
	p := r.URL.Query().Get("path")
	var data any
	var n int64
	download := &fileDownloadWriter{w: w, name: path.Base(p)}
	switch op {
	case "list":
		data, err = files.List(ctx, p)
	case "preview":
		var text string
		text, err = files.Preview(ctx, p)
		data = map[string]string{"text": text}
	case "upload":
		n, err = files.Upload(ctx, p, http.MaxBytesReader(w, r.Body, sftpfiles.TransferLimit), sftpfiles.TransferLimit)
		data = map[string]int64{"bytes": n}
	case "download":
		n, err = files.Download(ctx, p, download)
	}
	web.auditFiles(r, target, v, "sftp_"+op, p, n, err)
	if err != nil {
		if download.started {
			panic(http.ErrAbortHandler)
		}
		fileError(w, err, 0)
		return
	}
	if op == "download" {
		download.headers()
		return
	}
	writeJSON(w, 200, map[string]any{"code": 200, "message": "success", "data": data})
}

type fileDownloadWriter struct {
	w       http.ResponseWriter
	name    string
	started bool
}

func (d *fileDownloadWriter) headers() {
	if d.started {
		return
	}
	d.started = true
	d.w.Header().Set("Content-Type", "application/octet-stream")
	d.w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": d.name}))
	d.w.WriteHeader(200)
}
func (d *fileDownloadWriter) Write(p []byte) (int, error) { d.headers(); return d.w.Write(p) }

func fileReason(err error) (int, string) {
	switch {
	case errors.Is(err, errUnauthorized):
		return 401, "SFTP_UNAUTHORIZED"
	case errors.Is(err, iam.ErrForbidden), errors.Is(err, os.ErrPermission), errors.Is(err, sftpfiles.ErrReadOnly):
		return 403, "SFTP_FORBIDDEN"
	case errors.Is(err, errFileSessionMissing), errors.Is(err, os.ErrNotExist):
		return 404, "SFTP_NOT_FOUND"
	case errors.Is(err, errFileSessionBusy), errors.Is(err, sftpfiles.ErrBusy):
		return 409, "SFTP_BUSY"
	case errors.Is(err, os.ErrExist):
		return 409, "SFTP_EXISTS"
	case errors.Is(err, sftpfiles.ErrInvalidPath), errors.Is(err, sftpfiles.ErrNotRegular):
		return 400, "SFTP_INVALID"
	case errors.Is(err, sftpfiles.ErrTooLarge):
		return 413, "SFTP_TOO_LARGE"
	case errors.Is(err, sftpfiles.ErrBinary):
		return 415, "SFTP_BINARY"
	case errors.Is(err, sftpfiles.ErrAtomicUpload):
		return 409, "SFTP_UPLOAD_UNSUPPORTED"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return 408, "SFTP_TIMEOUT"
	default:
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return 413, "SFTP_TOO_LARGE"
		}
		return 502, "SFTP_FAILED"
	}
}
func fileError(w http.ResponseWriter, err error, status int) {
	code, reason := fileReason(err)
	if status != 0 {
		code = status
	}
	id, _ := randomWebSSHToken()
	writeJSON(w, code, map[string]any{"code": code, "message": reason, "reason": reason, "request_id": id})
}
func (web *web) auditFiles(r *http.Request, target *controlplane.WebSSHTarget, v fileSession, op, p string, n int64, err error) {
	reason := ""
	if err != nil {
		_, reason = fileReason(err)
	}
	web.recordWebSSHAudit(target, v.userID, iam.ExtractClientIP(r), "request", op, v.username, fmt.Sprintf("session=%s path=%q bytes=%d", v.id, p, n), err == nil, 0, reason)
}

var _ io.Writer = (*fileDownloadWriter)(nil)
