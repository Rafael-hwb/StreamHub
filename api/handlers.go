package main

import (
	"github.com/Rafael-hwb/streamhub/api/dbops"
	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/session"

	"github.com/gin-gonic/gin"
)

func CreateUser(context *gin.Context){
	userBody := &defs.UserCredential{}
	if err := context.ShouldBindJSON(userBody); err != nil{
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	if err := dbops.AddCredential(userBody.UserName, userBody.Pwd); err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	sid := session.GenerateSessionId(userBody.UserName)
	signUpMessage := &defs.SignUp{Success: true, SessionId: sid}

	SendNormalResponse(context, 201, signUpMessage)
}

func Login(context *gin.Context){
	userBody := &defs.UserCredential{}
	if err := context.ShouldBindJSON(userBody); err != nil{
		SendErrorResponse(context, defs.ErrorRequestBodyParseFailed)
		return
	}

	pwd, err := dbops.GetCredential(userBody.UserName)
	if err != nil{
		SendErrorResponse(context, defs.ErrorDBError)
		return
	}

	if len(pwd) == 0{
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return
	}

	if pwd != userBody.Pwd{
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return
	}

	sid := session.GenerateSessionId(userBody.UserName)
	signUpMessage := &defs.SignUp{Success: true, SessionId: sid}

	SendNormalResponse(context, 200, signUpMessage)
}
