package main

import (
	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/Rafael-hwb/streamhub/internal/httpx"
	"github.com/gin-gonic/gin"
)

func LimiterMiddleware(maxCount int) gin.HandlerFunc {
	limiter := CreateConnectionLimiter(maxCount)
	return func(context *gin.Context) {
		if !limiter.GetConnection() {
			context.Error(errs.TooManyRequests("Too many requests."))
			return
		}
		defer limiter.ReleaseConnection()
		context.Next()
	}
}

func RegisterHandlers() *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), httpx.ErrorHandler())

	router.Use(LimiterMiddleware(10))
	router.Use(CorsMiddleware())

	router.GET("/videos/:vid-id", StreamHandler)
	router.POST("/upload/:vid-id", UploadHandler)

	router.GET("/testpage", TestPageHandler)

	return router
}

func main() {
	router := RegisterHandlers()

	router.Run(":9000")
}
