package main

import (
	"github.com/Rafael-hwb/streamhub/scheduler/dbops"
	"github.com/gin-gonic/gin"
)

func videoDelRecHandler(context *gin.Context) {
	vid := context.Param("vid-id")

	if len(vid) == 0 {
		SendErrorResponse(context, 400, "Video id should not be empty.")
		return
	}

	err := dbops.AddVideoDeletionRecord(vid)
	if err != nil {
		SendErrorResponse(context, 500, "Internal server error.")
		return
	}
	SendNormalResponse(context, 200, "")
	return
}
