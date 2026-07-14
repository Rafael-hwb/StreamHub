package main
import "github.com/gin-gonic/gin"

func RegisterHandlers() *gin.Engine {
	router := gin.Default()

	router.POST("/user", CreateUser)
	router.POST("/user/:user_name", Login)

	return router
}


func main(){
	router := RegisterHandlers()
	router.Run(":8080")
}