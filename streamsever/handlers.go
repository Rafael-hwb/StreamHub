package main

import (
	"html/template"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPageHandler(context *gin.Context) {
	t, err := template.ParseFiles("./videos/upload.html")
	if err != nil {
		SendErrorResponse(context, ErrorInternalFaults)
		return
	}

	err = t.Execute(context.Writer, nil)
	if err != nil {
		SendErrorResponse(context, ErrorInternalFaults)
		return
	}
}

func StreamHandler(context *gin.Context) {
	vid := context.Param("vid-id")
	videoLink := VIDEO_DIR + vid

	video, err := os.Open(videoLink)
	if err != nil {
		SendErrorResponse(context, ErrorInternalFaults)
		return
	}
	http.ServeContent(context.Writer, context.Request, "", time.Now(), video)

	defer video.Close()
}

func UploadHandler(context *gin.Context) {
	err := context.Request.ParseMultipartForm(MAX_UPLOAD_SIZE)
	if err != nil {
		SendErrorResponse(context, ErrorFileTooBig)
		return
	}

	vid := context.Param("vid-id")

	file, err := context.FormFile("file")
	if err != nil {
		SendErrorResponse(context, ErrorRequestError)
		return
	}

	err = context.SaveUploadedFile(file, VIDEO_DIR+vid)
	if err != nil {
		SendErrorResponse(context, ErrorInternalFaults)
		return
	}

	SendNormalResponse(context, http.StatusCreated, gin.H{
		"success": true,
	})
}
