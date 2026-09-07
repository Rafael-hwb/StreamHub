package main

import "github.com/gin-gonic/gin"

func CorsMiddleware() gin.HandlerFunc {
	return func(context *gin.Context) {
		context.Header("Access-Control-Allow-Origin", "*")
		context.Header("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		context.Header("Access-Control-Allow-Headers", "Origin, Content-Type, X-Session-Id")
		if context.Request.Method == "OPTIONS" {
			context.AbortWithStatus(204)
			return
		}
		context.Next()
	}
}
