package web

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// saveWebSFTPConnectionHTTP saves connection metadata and write-only credentials.
// @Summary Save a user-owned WebSFTP connection
// @Router /api/v1/webssh/proxies/{id}/credential [post]
// @Router /api/v1/webssh/proxies/{id}/credential [put]
// @Success 200 {object} map[string]interface{}
func (web *web) saveWebSFTPConnectionHTTP(w http.ResponseWriter, r *http.Request, ctx context.Context, proxyID uint) {
	w.Header().Set("Cache-Control", "no-store")
	var req struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Password string `json:"password"`
		Remember bool   `json:"remember_password"`
	}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16*1024))
	dec.DisallowUnknownFields()
	if dec.Decode(&req) != nil || strings.TrimSpace(req.Name) == "" || len(req.Name) > 128 || strings.TrimSpace(req.Username) == "" || len(req.Username) > 255 {
		writeJSON(w, 400, map[string]any{"code": 400, "message": "invalid connection profile"})
		return
	}
	// Authorize before encrypting or revealing whether an account is already saved.
	if _, err := web.controlPlane.GetWebSSHTarget(ctx, proxyID); err != nil {
		writeJSON(w, 403, map[string]any{"code": 403, "message": "connection access denied"})
		return
	}
	encrypted, nonce := "", ""
	if req.Remember && req.Password != "" {
		var err error
		encrypted, nonce, err = web.encryptWebSSHPassword([]byte(req.Password))
		if err != nil {
			writeJSON(w, 500, map[string]any{"code": 500, "message": "credential storage unavailable"})
			return
		}
	}
	req.Password = ""
	err := web.controlPlane.SaveWebSFTPConnection(ctx, proxyID, req.Name, req.Username, encrypted, nonce, req.Remember, r.Method == http.MethodPost)
	if err != nil {
		writeJSON(w, 400, map[string]any{"code": 400, "message": "unable to save connection; check the account and password"})
		return
	}
	writeJSON(w, 200, map[string]any{"code": 200, "message": "success"})
}
