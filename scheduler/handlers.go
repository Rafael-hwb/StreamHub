package main

import (
	"net/http"

	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/Rafael-hwb/streamhub/internal/httpx"
	"github.com/Rafael-hwb/streamhub/scheduler/dbops"
	"github.com/gin-gonic/gin"
)

func videoDelRecHandler(context *gin.Context) {
	vid := context.Param("vid-id")
	if !validVideoID(vid){
		context.Error(errs.BadRequest("Invalid video id."))
	}
	
	if len(vid) == 0 {
		context.Error(errs.BadRequest("Video id should not be empty."))
		return
	}

	err := dbops.AddVideoDeletionRecord(vid)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}
	httpx.Success(context, http.StatusAccepted, nil)
	return
}
