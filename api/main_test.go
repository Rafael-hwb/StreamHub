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
