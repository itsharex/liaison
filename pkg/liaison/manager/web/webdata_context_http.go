package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/liaisonio/liaison/pkg/liaison/manager/iam"
)

// Navigation only: never use these fields to authorize or change a connection.
type dataWorkspaceContext struct {
	Database   string `json:"database"`
	Schema     string `json:"schema"`
	ObjectType string `json:"object_type"`
	Name       string `json:"name"`
}

func (v dataWorkspaceContext) valid() bool {
	for _, value := range []string{v.Database, v.Schema, v.Name} {
		if len(value) > 1024 || !utf8.ValidString(value) || strings.ContainsAny(value, "\x00\r\n") {
			return false
		}
	}
	switch v.ObjectType {
	case "":
		return v.Name == ""
	case "database", "schema", "table", "collection", "index", "key":
		return v.Name != ""
	default:
		return false
	}
}

// handleWebDataContextHTTP replaces the current user-scoped navigation hint.
// @Summary Synchronize database or cache Agent navigation context
// @Router /api/v1/webdata/sessions/{token}/context [post]
// @Success 200 {object} map[string]interface{}
func (web *web) handleWebDataContextHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeJSON(w, 405, map[string]any{"code": 405, "message": "method not allowed"})
		return
	}
	actor, err := web.authenticateHTTP(r)
	if err != nil {
		writeUnauthorized(w)
		return
	}
	if web.iamService.RequireFeature(actor, iam.FeatureAccessAI) != nil {
		writeJSON(w, 403, map[string]any{"code": 403, "message": "permission denied"})
		return
	}
	token, err := parseWebDataSessionToken(r, "/context")
	if err != nil {
		writeJSON(w, 400, map[string]any{"code": 400, "message": "invalid session"})
		return
	}
	session, ok := web.webData.get(token)
	if !ok || session.userID != actor.ID {
		writeUnauthorized(w)
		return
	}
	if web.ensureWebDataSessionActive(r.Context(), session) != nil {
		writeJSON(w, 409, map[string]any{"code": 409, "message": "session unavailable"})
		return
	}
	controller := http.NewResponseController(w)
	if controller.SetReadDeadline(time.Now().Add(10*time.Second)) == nil {
		defer controller.SetReadDeadline(time.Time{})
	}
	var value dataWorkspaceContext
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || !value.valid() {
		writeJSON(w, 400, map[string]any{"code": 400, "message": "invalid workspace context"})
		return
	}
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.protocol == "s3" {
		writeJSON(w, 400, map[string]any{"code": 400, "message": "use storage context"})
		return
	}
	session.dataContext = value
	writeJSON(w, 200, map[string]any{"code": 200, "message": "success"})
}
