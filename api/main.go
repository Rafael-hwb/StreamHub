package main

import (
	"github.com/Rafael-hwb/streamhub/api/defs"

	"github.com/gin-gonic/gin"
)

func SessionMiddleware(context *gin.Context){
	if !ValidateUserSession(context){
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return
	}
	context.Next()
}

func RegisterHandlers() *gin.Engine {
	router := gin.Default()

	router.POST("/user", CreateUser)
	router.POST("/user/login", Login)

	api := router.Group("/api")
	api.Use(SessionMiddleware)

	return router
}

func main(){
	router := RegisterHandlers()
	router.Run(":8080")
}
