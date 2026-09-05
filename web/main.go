package main

import(
	"github.com/gin-gonic/gin"
)

func RegisterRouter() *gin.Engine{
	router := gin.Default()

	router.GET(("/"), HomeHandler)
	router.POST("/", HomeHandler)

	router.GET("/userhome", UserHomeHandler)
	router.POST("/userhome", UserHomeHandler)

	router.POST("/api",ApiHandler)

	router.Static("/static", "./templetes")   
}

func main(){
	router := RegisterRouter()

	router.Run(":8080")
}