package main

import (
	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/session"

	"github.com/gin-gonic/gin"
)

var HEADER_FIELD_SESSION = "X-Session-Id"
var HEADER_FIELD_USERNAME = "X-User-Name"

func ValidateUserSession(context *gin.Context) bool{
	sid := context.GetHeader(HEADER_FIELD_SESSION)

	if len(sid) == 0{
		return false
	}

	username, isValid := session.IsSessionValid(sid)
	if isValid{
		context.Request.Header.Set(HEADER_FIELD_USERNAME, username)
		return true
	}

	return false
}

func ValidateUser(context *gin.Context) bool{
	username := context.GetHeader(HEADER_FIELD_USERNAME)

	if len(username) == 0{
		SendErrorResponse(context, defs.ErrorNotAuthUser)
		return false
	}

	return true
}
