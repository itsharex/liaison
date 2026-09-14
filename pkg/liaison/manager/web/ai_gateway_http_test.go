package web

import (
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/liaisonio/liaison/pkg/liaison/manager/aigateway"
	"github.com/liaisonio/liaison/pkg/liaison/manager/controlplane"
	"github.com/liaisonio/liaison/pkg/liaison/manager/iam"
	"github.com/stretchr/testify/require"
)

func TestAIDecodeBoundsAndStrictConfig(t *testing.T) {
	for _, body := range []string{`{"enabled":true} {}`, `{"enabled":true,"unknown":true}`, `{"enabled":"true"}`, strings.Repeat(" ", 1<<20) + `{}`} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("PUT", "/api/v1/ai/accesses/1", strings.NewReader(body))
		var c controlplane.AIAccessConfig
		require.ErrorIs(t, aiDecode(w, r, &c), controlplane.ErrAIInvalid)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest("PUT", "/api/v1/ai/accesses/1", strings.NewReader(`{"enabled":true,"models":{"chat":"internal"}}`))
	var c controlplane.AIAccessConfig
	require.NoError(t, aiDecode(w, r, &c))
	require.True(t, c.Enabled)
}

func TestAIErrorsNeverExposeUnderlyingDetails(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{iam.ErrForbidden, 403}, {controlplane.ErrAIInvalid, 400},
		{controlplane.ErrAIUnavailable, 503}, {aigateway.ErrModelDenied, 403},
		{aigateway.ErrUnsupported, 400}, {errors.New("secret upstream credential and prompt"), 500},
	} {
		w := httptest.NewRecorder()
		require.Equal(t, tc.status, aiStatus(tc.err))
		aiError(w, aiStatus(tc.err))
		require.NotContains(t, w.Body.String(), "credential")
		require.NotContains(t, w.Body.String(), "prompt")
	}
}

func TestAIGatewayUnconfiguredFailsClosed(t *testing.T) {
	w := httptest.NewRecorder()
	(&web{}).handleAIGatewayHTTP(w, httptest.NewRequest("GET", "/api/v1/ai/accesses/1/v1/models", nil))
	require.Equal(t, 503, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
}

func TestAIErrorCarriesSafeReasonAndRequestID(t *testing.T) {
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "test-request")
	aiError(w, 502, "UPSTREAM_AUTHENTICATION_FAILED")
	require.Equal(t, 502, w.Code)
	require.Contains(t, w.Body.String(), "UPSTREAM_AUTHENTICATION_FAILED")
	require.Contains(t, w.Body.String(), "test-request")
}

func TestAIInferenceKeyRejectsAmbiguousCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, key, authorization string
		native, want             bool
	}{
		{"native key", "test-key", "", true, true},
		{"not accepted on OpenAI", "test-key", "", false, false},
		{"ambiguous", "test-key", "Bearer other", true, false},
		{"whitespace", " test-key", "", true, false},
		{"bearer fallback", "", "Bearer test-key", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "/", nil)
			if tc.key != "" {
				r.Header.Set("x-api-key", tc.key)
			}
			if tc.authorization != "" {
				r.Header.Set("Authorization", tc.authorization)
			}
			_, ok := aiInferenceKey(r, tc.native)
			require.Equal(t, tc.want, ok)
		})
	}
	r := httptest.NewRequest("POST", "/", nil)
	r.Header.Add("x-api-key", "one")
	r.Header.Add("x-api-key", "two")
	_, ok := aiInferenceKey(r, true)
	require.False(t, ok)
}
