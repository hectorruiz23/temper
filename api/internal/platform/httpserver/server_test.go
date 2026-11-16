package httpserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "test-token-value"

type errorResponse struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func call(t *testing.T, handler http.Handler, method, path, token string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(method, path, nil)
	if token != "" {
		request.Header.Set("X-Temper-Token", token)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	return recorder
}

func requireError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string) errorResponse {
	t.Helper()

	if recorder.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, status, recorder.Body.String())
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type = %q, want application/json", contentType)
	}

	var response errorResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode error body %q: %v", recorder.Body.String(), err)
	}
	if response.Error.Code != code {
		t.Fatalf("code = %q, want %q", response.Error.Code, code)
	}
	if response.Error.Message == "" {
		t.Fatal("error message is empty")
	}

	return response
}

func TestHealthIsPublic(t *testing.T) {
	handler := NewHandler(testToken)

	recorder := call(t, handler, http.MethodGet, "/api/health", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
	if body := strings.TrimSpace(recorder.Body.String()); body != `{"ok":true}` {
		t.Fatalf("body = %q, want {\"ok\":true}", body)
	}
	if contentType := recorder.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("content type = %q, want application/json", contentType)
	}
}

func TestHealthWithValidToken(t *testing.T) {
	recorder := call(t, NewHandler(testToken), http.MethodGet, "/api/health", testToken)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestProtectedRoutesRejectMissingOrWrongToken(t *testing.T) {
	handler := NewHandler(testToken)

	tests := []struct {
		name   string
		method string
		path   string
	}{
		{name: "unknown path without token", method: http.MethodGet, path: "/api/plans/current"},
		{name: "root without token", method: http.MethodGet, path: "/"},
		{name: "health trailing slash without token", method: http.MethodGet, path: "/api/health/"},
		{name: "wrong method on health without token", method: http.MethodPost, path: "/api/health"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := call(t, handler, test.method, test.path, "")
			requireError(t, recorder, http.StatusUnauthorized, "unauthorized")

			recorder = call(t, handler, test.method, test.path, "wrong-token")
			requireError(t, recorder, http.StatusUnauthorized, "unauthorized")
		})
	}
}

func TestUnauthorizedBodyIsFixed(t *testing.T) {
	handler := NewHandler(testToken)

	missing := call(t, handler, http.MethodGet, "/api/plans/current", "")
	wrong := call(t, handler, http.MethodGet, "/api/plans/current", "wrong-token")

	const want = `{"error":{"code":"unauthorized","message":"missing or invalid token"}}`
	if body := strings.TrimSpace(missing.Body.String()); body != want {
		t.Fatalf("missing-token body = %q, want %q", body, want)
	}
	if missing.Body.String() != wrong.Body.String() {
		t.Fatalf("missing-token body %q differs from wrong-token body %q", missing.Body.String(), wrong.Body.String())
	}
}

func TestValidTokenOnUnknownRouteIsNotFound(t *testing.T) {
	handler := NewHandler(testToken)

	tests := []string{"/api/plans/current", "/", "/api/health/"}
	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			recorder := call(t, handler, http.MethodGet, path, testToken)
			requireError(t, recorder, http.StatusNotFound, "not_found")
		})
	}
}

func TestWrongMethodWithValidTokenIsNotFound(t *testing.T) {
	recorder := call(t, NewHandler(testToken), http.MethodPost, "/api/health", testToken)
	requireError(t, recorder, http.StatusNotFound, "not_found")
}

func TestTokenIsNeverEchoed(t *testing.T) {
	handler := NewHandler(testToken)

	responses := []*httptest.ResponseRecorder{
		call(t, handler, http.MethodGet, "/api/health", testToken),
		call(t, handler, http.MethodGet, "/api/plans/current", testToken),
		call(t, handler, http.MethodGet, "/api/plans/current", "wrong-token"),
		call(t, handler, http.MethodGet, "/api/plans/current", ""),
	}

	for _, recorder := range responses {
		if strings.Contains(recorder.Body.String(), testToken) {
			t.Fatalf("response body echoes the token: %q", recorder.Body.String())
		}
		for name, values := range recorder.Header() {
			for _, value := range values {
				if strings.Contains(value, testToken) {
					t.Fatalf("response header %s echoes the token", name)
				}
			}
		}
	}
}
