package httpx

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/gin-gonic/gin"
)

func ErrorHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if len(c.Errors) == 0 {
			return
		}
		err := c.Errors.Last().Err

		var appError *errs.AppError

		if errors.As(err, &appError) {
			c.AbortWithStatusJSON(appError.HTTPStatus, Response{
				Code:    appError.Code,
				Message: appError.Message,
				Data:    nil,
			})

			if appError.Cause != nil {
				slog.Error("request failed", "path", c.Request.URL.Path, "cause", appError.Cause)
			}
			return
		}

		slog.Error("unhandled error", "path", c.Request.URL.Path, "err", err)
		c.AbortWithStatusJSON(http.StatusInternalServerError, Response{
			Code:    "INTERNAL",
			Message: "Internal server error",
			Data:    nil,
		})
	}
}
