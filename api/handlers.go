package main

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/Rafael-hwb/streamhub/api/dbops"
	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/session"
	"github.com/Rafael-hwb/streamhub/internal/errs"
	"github.com/Rafael-hwb/streamhub/internal/httpx"
	"github.com/gin-gonic/gin"
)

func CreateUser(context *gin.Context) {
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

	if err := dbops.AddCredential(userBody.UserName, userBody.Pwd); err != nil {
		context.Error(errs.Internal(err))
		return
	}

	sid := session.GenerateSessionId(userBody.UserName)
	signUpMessage := &defs.SignUp{Success: true, SessionId: sid}

	httpx.Success(context, http.StatusCreated, signUpMessage)
}


func CreateVideoInfo(context *gin.Context) {
	aid, err := currentUserID(context)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}


	videoBody := &defs.VideoCreateRequest{}
	if err := context.ShouldBindJSON(videoBody); err != nil{
		context.Error(errs.BadRequest("Request body is invalid."))
		return
	}

	if len(videoBody.Title) == 0{
		context.Error(errs.BadRequest("Video title is required."))
		return
	}

	videoInfo, err := dbops.AddVideo(aid, videoBody.Title)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusCreated, videoInfo)
}


func Login(context *gin.Context) {
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
		
	pwd, err := dbops.GetCredential(userBody.UserName)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}

	if len(pwd) == 0 {
		context.Error(errs.Unauthorized("Invalid user name or password."))
		return
	}

	if pwd != userBody.Pwd {
		context.Error(errs.Unauthorized("Invalid user name or password."))
		return
	}

	sid := session.GenerateSessionId(userBody.UserName)
	signUpMessage := &defs.SignUp{Success: true, SessionId: sid}

	httpx.Success(context, http.StatusOK, signUpMessage)
}


func MyVideos(context *gin.Context) {
	aid, err := currentUserID(context)

	if err != nil {
		context.Error(errs.Internal(err))
		return
	}


	videos, err := dbops.ListVideosByAuthor(aid)
	if err != nil{
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusOK, videos)
}


func GetVideoInfo(context *gin.Context) {
	vid := context.Param("vid")

	video, err := dbops.GetVideoDetail(vid)
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


func ListCommentsHandler(context *gin.Context){
	vid := context.Param("vid")

	comments, err := dbops.ListCommentsByVideo(vid)
	if err != nil{
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusOK, comments)
}

func AddCommentHandler(context *gin.Context){
	vid := context.Param("vid")
	
	video, err := dbops.GetVideo(vid)
	if err != nil{
		context.Error(errs.Internal(err))
		return
	}

	if video == nil{
		context.Error(errs.NotFound("Video not found."))
		return
	}

	aid, err := currentUserID(context)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}


	commentBody := &defs.CommentCreateRequest{}
	if err := context.ShouldBindJSON(commentBody); err != nil{
		context.Error(errs.BadRequest("Request body is invalid."))
		return
	}

	if len(commentBody.Content) == 0{
		context.Error(errs.BadRequest("Comment content is required."))
		return
	}

	err = dbops.AddComment(vid, aid, commentBody.Content)
	if err != nil{
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusCreated, commentBody)

}

func DeleteVideoHandler(context *gin.Context){
	vid := context.Param("vid")

	aid, err := currentUserID(context)
	if err != nil {
		context.Error(errs.Internal(err))
		return
	}


	video, err := dbops.GetVideo(vid)
	if err != nil{
		context.Error(errs.Internal(err))
		return
	}

	if video == nil{
		context.Error(errs.NotFound("Video not found."))
		return
	}

	if video.AuthorId != aid {
		context.Error(errs.Forbidden("You are not allowed to delete this video."))
		return
	}

	if err := dbops.DeleteVideo(vid); err != nil{
		context.Error(errs.Internal(err))
		return
	}
	
	resp, err := http.Get("http://localhost:9001/video-del-rec/" + vid)
	if err != nil {
		log.Printf("Notify scheduler error: %v", err)
	} else {
		resp.Body.Close()
	}

	httpx.Success(context, http.StatusOK, nil)
}


func ListVideosHandler(context *gin.Context){
	videos, err := dbops.ListAllVideos()
	if err != nil{
		context.Error(errs.Internal(err))
		return
	}

	httpx.Success(context, http.StatusOK, videos)
}


func currentUserID(context *gin.Context) (int, error) {
	username := context.GetHeader(HEADER_FIELD_USERNAME)

	aid, err := dbops.GetUserIDByName(username)
	if err != nil {
		return 0, err
	}
	if aid == 0 {
		return 0, fmt.Errorf("authenticated user %q not found", username)
	}

	return aid, nil
}