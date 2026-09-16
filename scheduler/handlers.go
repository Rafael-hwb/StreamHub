package main

import (
	"net/http"

	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/Rafael-hwb/streamhub/internal/httpx"
	"github.com/Rafael-hwb/streamhub/scheduler/dbops"
	"github.com/gin-gonic/gin"
)

func videoDelRecHandler(store *dbops.Store) gin.HandlerFunc {
	return func(context *gin.Context) {
		vid := context.Param("vid-id")
		if !ValidVideoID(vid) {
			context.Error(errs.BadRequest("Invalid video id."))
			return
		}

		if len(vid) == 0 {
			context.Error(errs.BadRequest("Video id should not be empty."))
			return
		}

		if err := store.AddVideoDeletionRecord(vid); err != nil {
			context.Error(errs.Internal(err))
			return
		}
		httpx.Success(context, http.StatusAccepted, nil)
	}
}
