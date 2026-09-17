package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type testResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func decodeResponse(t *testing.T, response *httptest.ResponseRecorder) testResponse {
	t.Helper()

	var body testResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, response.Body.String())
	}
	return body
}

// newTestRouter 构造测试路由。这些测试只覆盖输入校验路径，
// 校验在访问 store 之前就返回，故传 nil store 是安全的。
func newTestRouter() *gin.Engine {
	return NewHandler(nil, nil).RegisterHandlers()
}

func TestProtectedRejectMissingSession(t *testing.T) {
	router := newTestRouter()

	request := httptest.NewRequest(http.MethodGet, "/api/my/videos", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401, got %d; body=%s", response.Code, response.Body.String())
	}

	if !strings.Contains(response.Body.String(), `"code":"UNAUTHORIZED"`) {
		t.Fatalf("expected UNAUTHORIZED response, got %s", response.Body.String())
	}
}

func TestCredentialEndpointsRejectInvalidInput(t *testing.T) {
	router := newTestRouter()

	tests := []struct {
		name    string
		path    string
		body    string
		message string
	}{
		{
			name:    "register malformed json",
			path:    "/user",
			body:    `{"user_name":`,
			message: "Request body is invalid.",
		},
		{
			name:    "register missing username",
			path:    "/user",
			body:    `{"user_name":"   ","pwd":"123456"}`,
			message: "User name is required.",
		},
		{
			name:    "register missing password",
			path:    "/user",
			body:    `{"user_name":"rafael","pwd":""}`,
			message: "Password is required.",
		},
		{
			name:    "login malformed json",
			path:    "/user/login",
			body:    `{"user_name":`,
			message: "Request body is invalid.",
		},
		{
			name:    "login missing username",
			path:    "/user/login",
			body:    `{"user_name":"   ","pwd":"123456"}`,
			message: "User name is required.",
		},
		{
			name:    "login missing password",
			path:    "/user/login",
			body:    `{"user_name":"rafael","pwd":""}`,
			message: "Password is required.",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(
				http.MethodPost,
				test.path,
				strings.NewReader(test.body),
			)
			request.Header.Set("Content-Type", "application/json")

			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			body := decodeResponse(t, response)

			if body.Code != "BAD_REQUEST" {
				t.Fatalf("expected code BAD_REQUEST, got %q", body.Code)
			}

			if body.Message != test.message {
				t.Fatalf("expected message %q, got %q", test.message, body.Message)
			}
		})
	}
}
