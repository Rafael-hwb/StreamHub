package main

import (
	"github.com/Rafael-hwb/streamhub/api/defs"

	"github.com/gin-gonic/gin"
)

func SendErrorResponse(context *gin.Context, errResponse defs.ErrResponse){
	context.AbortWithStatusJSON(errResponse.HttpSC, errResponse.Error)
}

func SendNormalResponse(context *gin.Context, statusCode int, response interface{}){
	context.JSON(statusCode, response)
}
