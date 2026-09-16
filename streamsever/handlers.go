package main

import (
	"html/template"
	"net/http"
	"os"
	"time"

	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/Rafael-hwb/streamhub/internal/httpx"
	"github.com/gin-gonic/gin"
)

func TestPageHandler(context *gin.Context) {
	t, err := template.ParseFiles("./videos/upload.html")
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	err = t.Execute(context.Writer, nil)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}
}

func StreamHandler(context *gin.Context) {
	vid := context.Param("vid-id")
	if !ValidVideoID(vid) {
		context.Error(errs.BadRequest("Invalid video id."))
		return
	}
	videoLink, err := SafePath(VIDEO_DIR, vid)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	video, err := os.Open(videoLink)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}
	http.ServeContent(context.Writer, context.Request, "", time.Now(), video)

	defer video.Close()
}

func UploadHandler(context *gin.Context) {
	err := context.Request.ParseMultipartForm(MAX_UPLOAD_SIZE)
	if err != nil {
		context.Error(errs.BadRequest("File is too big."))
		return
	}

	vid := context.Param("vid-id")
	if !ValidVideoID(vid) {
		context.Error(errs.BadRequest("Invalid video id."))
		return
	}

	file, err := context.FormFile("file")
	if err != nil {
		context.Error(errs.BadRequest("Request is wrong."))
		return
	}

	err = context.SaveUploadedFile(file, VIDEO_DIR+vid)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusAccepted, nil)
}
