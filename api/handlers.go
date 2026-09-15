package main

import (
	"log"
	"net/http"
	"strings"

	"github.com/Rafael-hwb/streamhub/api/dbops"
	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/session"
	"github.com/Rafael-hwb/streamhub/internal/errs"
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
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	sid := session.GenerateSessionId(userBody.UserName)
	signUpMessage := &defs.SignUp{Success: true, SessionId: sid}

	SendNormalResponse(context, 201, signUpMessage)
}


func CreateVideoInfo(context *gin.Context) {
	username := context.GetHeader(HEADER_FIELD_USERNAME)
	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0 {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	videoBody := &defs.VideoCreateRequest{}
	if err := context.ShouldBindJSON(videoBody); err != nil{
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	if len(videoBody.Title) == 0{
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	videoInfo, err := dbops.AddVideo(aid, videoBody.Title)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 201, videoInfo)
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
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	if len(pwd) == 0 {
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return
	}

	if pwd != userBody.Pwd {
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return
	}

	sid := session.GenerateSessionId(userBody.UserName)
	signUpMessage := &defs.SignUp{Success: true, SessionId: sid}

	SendNormalResponse(context, 200, signUpMessage)
}


func MyVideos(context *gin.Context) {
	username := context.GetHeader(HEADER_FIELD_USERNAME)
	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0 {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	videos, err := dbops.ListVideosByAuthor(aid)
	if err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 200, videos)
}


func GetVideoInfo(context *gin.Context) {
	vid := context.Param("vid")

	video, err := dbops.GetVideoDetail(vid)
	if err != nil {
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}
	if video == nil {
		SendErrorResponse(context, defs.ErrorVideoNotFound)
		return
	}

	SendNormalResponse(context, 200, video)
}


func ListCommentsHandler(context *gin.Context){
	vid := context.Param("vid")

	comments, err := dbops.ListCommentsByVideo(vid)
	if err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 200, comments)
}

func AddCommentHandler(context *gin.Context){
	vid := context.Param("vid")
	username := context.GetHeader(HEADER_FIELD_USERNAME)
	
	video, err := dbops.GetVideo(vid)
	if err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	if video == nil{
		SendErrorResponse(context, defs.ErrorVideoNotFound)
		return
	}

	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	commentBody := &defs.CommentCreateRequest{}
	if err := context.ShouldBindJSON(commentBody); err != nil{
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	if len(commentBody.Content) == 0{
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	err = dbops.AddComment(vid, aid, commentBody.Content)
	if err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 201, gin.H{"success":true})

}

func DeleteVideoHandler(context *gin.Context){
	vid := context.Param("vid")
	username := context.GetHeader(HEADER_FIELD_USERNAME)

	aid, err := dbops.GetUserIDByName(username)
	if err != nil || aid == 0{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	video, err := dbops.GetVideo(vid)
	if err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	if video == nil{
		SendErrorResponse(context, defs.ErrorVideoNotFound)
		return
	}

	if video.AuthorId != aid {
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return
	}

	if err := dbops.DeleteVideo(vid); err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}
	
	resp, err := http.Get("http://localhost:9001/video-del-rec/" + vid)
	if err != nil {
		log.Printf("Notify scheduler error: %v", err)
	} else {
		resp.Body.Close()
	}

	SendNormalResponse(context, 200, gin.H{"success": true})
}


func ListVideosHandler(context *gin.Context){
	videos, err := dbops.ListAllVideos()
	if err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	SendNormalResponse(context, 200, videos)
}