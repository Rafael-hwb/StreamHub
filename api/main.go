package main

import (
	"errors"
	"log"
	"os"

	"github.com/Rafael-hwb/streamhub/api/dbops"
	"github.com/Rafael-hwb/streamhub/api/session"
	"github.com/Rafael-hwb/streamhub/internal/config"
	"github.com/Rafael-hwb/streamhub/internal/dbconn"
	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/Rafael-hwb/streamhub/internal/httpx"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func (h *Handler) SessionMiddleware(context *gin.Context) {
	if !h.ValidateUserSession(context) {
		context.Error(errs.Unauthorized("User is not authenticated."))
		context.Abort()
		return
	}
	context.Next()
}

func (h *Handler) RegisterHandlers() *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), httpx.ErrorHandler())

	router.POST("/user", h.CreateUser)
	router.POST("/user/login", h.Login)

	router.Static("/static", "./static")
	router.StaticFile("/", "./static/index.html")
	router.StaticFile("/userhome", "./static/userhome.html")
	router.StaticFile("/video.html", "./static/video.html")

	publicAPI := router.Group("/api")
	publicAPI.GET("/videos", h.ListVideosHandler)
	publicAPI.GET("videos/:vid", h.GetVideoInfo)
	publicAPI.GET("/videos/:vid/comments", h.ListCommentsHandler)

	authAPI := router.Group("/api")
	authAPI.Use(h.SessionMiddleware)
	authAPI.POST("/videos", h.CreateVideoInfo)
	authAPI.GET("/my/videos", h.MyVideos)
	authAPI.POST("/videos/:vid/comments", h.AddCommentHandler)
	authAPI.DELETE("/videos/:vid", h.DeleteVideoHandler)

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

	db, err := dbconn.Open(cfg.MySQL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	store := dbops.NewStore(db)

	if err := session.LoadSessionsFromDB(store); err != nil {
		log.Printf("warning: load sessions from db: %v", err)
	}

	h := NewHandler(store)
	router := h.RegisterHandlers()
	router.Run(":8080")
}
