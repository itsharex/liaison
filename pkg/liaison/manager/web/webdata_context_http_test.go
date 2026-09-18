package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/manager/controlplane"
	"github.com/liaisonio/liaison/pkg/liaison/manager/iam"
	"github.com/liaisonio/liaison/pkg/liaison/repo/model"
	"github.com/stretchr/testify/require"
)

func TestDataContextIsolationReplacementAndSchema(t *testing.T) {
	w, admin, user := newPermissionHTTPTest(t)
	cp := &storageHTTPControl{target: &controlplane.WebDataTarget{ProxyID: 1, Protocol: "memcached", EffectiveStatus: "active"}}
	w.controlPlane = cp
	w.webData = newWebDataSessionStore()
	s, err := w.webData.create(&webDataSession{userID: user.ID, proxyID: 1, protocol: "memcached", database: "original", target: cp.target})
	require.NoError(t, err)
	call := func(actor *model.User, body string) int {
		r := httptest.NewRequest("POST", "/api/v1/webdata/sessions/"+s.token+"/context", strings.NewReader(body))
		if actor != nil {
			r = r.WithContext(context.WithValue(r.Context(), "user", actor))
		}
		out := httptest.NewRecorder()
		w.handleWebDataContextHTTP(out, r)
		return out.Code
	}
	require.Equal(t, 401, call(nil, `{}`))
	require.Equal(t, 403, call(user, `{}`))
	require.NoError(t, w.iamService.SetUserFeaturePolicy(admin, []string{iam.FeatureAccessAI}))
	require.Equal(t, 401, call(admin, `{}`))
	require.Equal(t, 200, call(user, `{"object_type":"key","name":"first"}`))
	require.Equal(t, 200, call(user, `{"database":"untrusted-other","object_type":"key","name":" second 中文 "}`))
	require.Equal(t, "original", s.database, "navigation must not switch the execution database")
	content, err := (&webDataAgentHandle{web: w, session: s}).Schema(context.Background(), nil)
	require.NoError(t, err)
	require.Contains(t, string(content), `"name":" second 中文 "`)
	require.NotContains(t, string(content), `"first"`)
	require.Contains(t, string(content), "untrusted browser navigation")
	for _, body := range []string{`{"statement":"SELECT secret"}`, `{"name":"x"}`, `{"object_type":"execute","name":"x"}`, `{} {}`, `{"name":"\u0000"}`, strings.Repeat(" ", 8193) + `{}`} {
		require.Equal(t, 400, call(user, body))
		require.Equal(t, " second 中文 ", s.dataContext.Name, "invalid updates must not replace valid context")
	}
	require.Equal(t, 200, call(user, `{}`))
	require.Equal(t, dataWorkspaceContext{}, s.dataContext)
	cp.target.EffectiveStatus = "stopped"
	require.Equal(t, 409, call(user, `{}`))
	cp.target.EffectiveStatus = "active"
	require.NoError(t, w.iamService.SetUserFeaturePolicy(admin, []string{}))
	require.Equal(t, 403, call(user, `{}`))
	require.NoError(t, w.iamService.SetUserFeaturePolicy(admin, []string{iam.FeatureAccessAI}))
	w.webData.delete(s.token)
	require.Equal(t, 401, call(user, `{}`))
}

func TestDataContextValidation(t *testing.T) {
	cases := []struct {
		name  string
		value dataWorkspaceContext
		valid bool
	}{
		{"empty selection", dataWorkspaceContext{}, true},
		{"table", dataWorkspaceContext{Database: "demo", Schema: "public", ObjectType: "table", Name: "orders"}, true},
		{"exact key", dataWorkspaceContext{ObjectType: "key", Name: " key 中文 "}, true},
		{"limit", dataWorkspaceContext{ObjectType: "key", Name: strings.Repeat("x", 1024)}, true},
		{"oversize", dataWorkspaceContext{ObjectType: "key", Name: strings.Repeat("x", 1025)}, false},
		{"invalid utf8", dataWorkspaceContext{ObjectType: "key", Name: string([]byte{255})}, false},
		{"control character", dataWorkspaceContext{Database: "a\nb"}, false},
		{"unknown type", dataWorkspaceContext{ObjectType: "query", Name: "secret"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { require.Equal(t, tc.valid, tc.value.valid()) })
	}
}
