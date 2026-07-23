package main

import (
	"github.com/gin-gonic/gin"
)

func LimiterMiddleware(maxCount int) gin.HandlerFunc {
	limiter := CreateConnectionLimiter(maxCount)
	return func(context *gin.Context) {
		if !limiter.GetConnection() {
			SendErrorResponse(context, ErrorTooManyRequests)
			return
		}
		defer limiter.ReleaseConnection()
		context.Next()
	}
}

func RegisterHandlers() *gin.Engine {
	router := gin.Default()

	router.Use(LimiterMiddleware(10))

	router.GET("/vedios/:vid-id", StreamHandler)
	router.POST("/upload/:vid-id", UploadHandler)

	return router
}

func main() {
	router := RegisterHandlers()

	router.Run(":9000")
}
