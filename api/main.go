package main

import (
	"errors"
	"log"
	"os"

	"github.com/Rafael-hwb/streamhub/api/dbops"
	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/internal/config"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func SessionMiddleware(context *gin.Context) {
	if !ValidateUserSession(context) {
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return
	}
	context.Next()
}

func RegisterHandlers() *gin.Engine {
	router := gin.Default()

	router.POST("/user", CreateUser)
	router.POST("/user/login", Login)

	router.Static("/static", "./static")
	router.StaticFile("/", "./static/index.html")
	router.StaticFile("/userhome", "./static/userhome.html")
	router.StaticFile("/video.html", "./static/video.html")


	publicAPI := router.Group("/api")
	publicAPI.GET("/videos", ListVideosHandler)
	publicAPI.GET("videos/:vid", GetVideoInfo)
	publicAPI.GET("/videos/:vid/comments", ListCommentsHandler)

	authAPI := router.Group("/api")
	authAPI.Use(SessionMiddleware)
	authAPI.POST("/videos", CreateVideoInfo)
	authAPI.GET("/my/videos", MyVideos)
	authAPI.POST("/videos/:vid/comments", AddCommentHandler)
	authAPI.DELETE("/videos/:vid", DeleteVideoHandler)

	return router
}

func main() {
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("warning: load .env: %v", err)
	}

	cfg, err := config.Load()

	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if err := dbops.Init(*cfg); err != nil {
		log.Fatalf("init db: %v", err)
	}

	router := RegisterHandlers()
	router.Run(":8080")
}
