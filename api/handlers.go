package main

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/Rafael-hwb/streamhub/api/dbops"
	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/session"
	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/Rafael-hwb/streamhub/internal/httpx"
	"github.com/Rafael-hwb/streamhub/scheduler/schedulerclient"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	store *dbops.Store
	scheduler *schedulerclient.Client
}

func NewHandler(store *dbops.Store, scheduler *schedulerclient.Client) *Handler {
	return &Handler{
		store: store,
		scheduler: scheduler,
	}
}

func (h *Handler) CreateUser(context *gin.Context) {
	userBody := &defs.UserCredential{}
	if err := context.ShouldBindJSON(userBody); err != nil {
		context.Error(errs.BadRequest("Request body is invalid."))
		return
	}

	if strings.TrimSpace(userBody.UserName) == "" {
		context.Error(errs.BadRequest("User name is required."))
		return
	}

	if userBody.Pwd == "" {
		context.Error(errs.BadRequest("Password is required."))
		return
	}

	if err := h.store.AddCredential(userBody.UserName, userBody.Pwd); err != nil {
		context.Error(errs.Internal(err))
		return
	}

	sid, err := session.GenerateSessionId(h.store, userBody.UserName)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}
	signUpMessage := &defs.SignUp{Success: true, SessionId: sid}

	httpx.Success(context, http.StatusCreated, signUpMessage)
}

func (h *Handler) CreateVideoInfo(context *gin.Context) {
	videoBody := &defs.VideoCreateRequest{}
	if err := context.ShouldBindJSON(videoBody); err != nil {
		context.Error(errs.BadRequest("Request body is invalid."))
		return
	}

	if len(videoBody.Title) == 0 {
		context.Error(errs.BadRequest("Video title is required."))
		return
	}

	aid, err := h.currentUserID(context)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	videoInfo, err := h.store.AddVideo(aid, videoBody.Title)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusCreated, videoInfo)
}

func (h *Handler) Login(context *gin.Context) {
	userBody := &defs.UserCredential{}
	if err := context.ShouldBindJSON(userBody); err != nil {
		context.Error(errs.BadRequest("Request body is invalid."))
		return
	}
	if strings.TrimSpace(userBody.UserName) == "" {
		context.Error(errs.BadRequest("User name is required."))
		return
	}
	if userBody.Pwd == "" {
		context.Error(errs.BadRequest("Password is required."))
		return
	}

	pwdHash, err := h.store.GetPasswordHash(userBody.UserName)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	if len(pwdHash) == 0 {
		context.Error(errs.Unauthorized("Invalid user name or password."))
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(pwdHash), []byte(userBody.Pwd)); err != nil {
		context.Error(errs.Unauthorized("Invalid user name or password."))
		return
	}

	sid, err := session.GenerateSessionId(h.store, userBody.UserName)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	signUpMessage := &defs.SignUp{Success: true, SessionId: sid}

	httpx.Success(context, http.StatusOK, signUpMessage)
}

func (h *Handler) MyVideos(context *gin.Context) {
	aid, err := h.currentUserID(context)

	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	videos, err := h.store.ListVideosByAuthor(aid)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusOK, videos)
}

func (h *Handler) GetVideoInfo(context *gin.Context) {
	vid := context.Param("vid")

	video, err := h.store.GetVideoDetail(vid)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}
	if video == nil {
		context.Error(errs.NotFound("Video not found."))
		return
	}

	httpx.Success(context, http.StatusOK, video)
}

func (h *Handler) ListCommentsHandler(context *gin.Context) {
	vid := context.Param("vid")

	comments, err := h.store.ListCommentsByVideo(vid)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusOK, comments)
}

func (h *Handler) AddCommentHandler(context *gin.Context) {
	vid := context.Param("vid")

	video, err := h.store.GetVideo(vid)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	if video == nil {
		context.Error(errs.NotFound("Video not found."))
		return
	}

	commentBody := &defs.CommentCreateRequest{}
	if err := context.ShouldBindJSON(commentBody); err != nil {
		context.Error(errs.BadRequest("Request body is invalid."))
		return
	}

	if len(commentBody.Content) == 0 {
		context.Error(errs.BadRequest("Comment content is required."))
		return
	}

	aid, err := h.currentUserID(context)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	err = h.store.AddComment(vid, aid, commentBody.Content)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusCreated, commentBody)

}

func (h *Handler) DeleteVideoHandler(context *gin.Context) {
	vid := context.Param("vid")

	aid, err := h.currentUserID(context)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	video, err := h.store.GetVideo(vid)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	if video == nil {
		context.Error(errs.NotFound("Video not found."))
		return
	}

	if video.AuthorId != aid {
		context.Error(errs.Forbidden("You are not allowed to delete this video."))
		return
	}

	if err := h.store.DeleteVideo(vid); err != nil {
		context.Error(errs.Internal(err))
		return
	}

	err = h.scheduler.NotifyVideoDeleted(context.Request.Context(), vid)

	if err != nil {
		context.Error(errs.Internal(err))
	}

	httpx.Success(context, http.StatusOK, nil)
}

func (h *Handler) ListVideosHandler(context *gin.Context) {
	videos, err := h.store.ListAllVideos()
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusOK, videos)
}

func (h *Handler) currentUserID(context *gin.Context) (int, error) {
	username := context.GetHeader(HEADER_FIELD_USERNAME)

	aid, err := h.store.GetUserIDByName(username)
	if err != nil {
		return 0, err
	}
	if aid == 0 {
		return 0, fmt.Errorf("authenticated user %q not found", username)
	}

	return aid, nil
}
