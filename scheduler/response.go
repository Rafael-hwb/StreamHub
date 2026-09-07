package main

import "github.com/gin-gonic/gin"

func SendErrorResponse(context *gin.Context, statusCode int, responseMessage string) {
	context.AbortWithStatusJSON(statusCode, responseMessage)
}

func SendNormalResponse(context *gin.Context, statusCode int, responseMessage string) {
	context.JSON(statusCode, responseMessage)
}
