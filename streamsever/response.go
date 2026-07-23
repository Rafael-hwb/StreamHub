package main

import (
	"github.com/gin-gonic/gin"
)

func SendErrorResponse(context *gin.Context, errResponse ErrResponse) {
	context.AbortWithStatusJSON(errResponse.HttpCode, errResponse.Err)
}

func SendNormalResponse(context *gin.Context, statusCode int, response interface{}) {
	context.JSON(statusCode, response)
}
