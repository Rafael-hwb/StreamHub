package main

import (
	"errors"
	"log"
	"os"

	"github.com/Rafael-hwb/streamhub/internal/config"
	"github.com/Rafael-hwb/streamhub/internal/dbconn"
	"github.com/Rafael-hwb/streamhub/internal/httpx"
	"github.com/Rafael-hwb/streamhub/scheduler/dbops"
	"github.com/Rafael-hwb/streamhub/scheduler/taskrunner"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func RegisterRouter(store *dbops.Store) *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery(), httpx.ErrorHandler())

	router.GET("/video-del-rec/:vid-id", videoDelRecHandler(store))

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

	go taskrunner.Start(store)

	router := RegisterRouter(store)

	router.Run(":9001")
}
