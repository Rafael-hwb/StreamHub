package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProtectedRejectMissingSession(t *testing.T) {
	router := RegisterHandlers()

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
	router := RegisterHandlers()

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

			if response.Code != http.StatusBadRequest {
				t.Fatalf(
					"expected status 400, got %d; body=%s",
					response.Code,
					response.Body.String(),
				)
			}

			if !strings.Contains(
				response.Body.String(),
				`"code":"BAD_REQUEST"`,
			) {
				t.Fatalf(
					"expected BAD_REQUEST response, got %s",
					response.Body.String(),
				)
			}

			if !strings.Contains(response.Body.String(), test.message) {
				t.Fatalf(
					"expected message %q, got %s",
					test.message,
					response.Body.String(),
				)
			}
		})
	}
}
