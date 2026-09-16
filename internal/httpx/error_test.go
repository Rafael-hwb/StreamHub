package httpx

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/gin-gonic/gin"
)

func TestErrorHandler(t *testing.T) {
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"bad request", errs.BadRequest("bad"), http.StatusBadRequest, "BAD_REQUEST"},
		{"unauthorized", errs.Unauthorized("unauth"), http.StatusUnauthorized, "UNAUTHORIZED"},
		{"forbidden", errs.Forbidden("forbidden"), http.StatusForbidden, "FORBIDDEN"},
		{"not found", errs.NotFound("not found"), http.StatusNotFound, "NOT_FOUND"},
		{"internal with cause", errs.Internal(errors.New("boom")), http.StatusInternalServerError, "INTERNAL"},
		{"plain error", errors.New("plain"), http.StatusInternalServerError, "INTERNAL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(ErrorHandler())
			router.GET("/test", func(c *gin.Context) {
				c.Error(tt.err)
			})

			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d; body=%s", tt.wantStatus, rec.Code, rec.Body.String())
			}

			var body Response
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
			}
			if body.Code != tt.wantCode {
				t.Fatalf("expected code %q, got %q", tt.wantCode, body.Code)
			}
		})
	}
}
