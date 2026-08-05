package main

import (
	"github.com/Rafael-hwb/streamhub/scheduler/taskrunner"
	"github.com/gin-gonic/gin"
)

func RegisterRouter() *gin.Engine{
	router := gin.Default()

	router.GET("/video-del-rec/:vid-id", videoDelRecHandler)

	return router
}

func main(){
	go taskrunner.Start()

	router := RegisterRouter()

	router.Run(":9001")
}