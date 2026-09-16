package httpx

import "github.com/gin-gonic/gin"

type Response struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data"`
}

func Success(c *gin.Context, status int, data any) {
	c.JSON(status, Response{
		Code:    "OK",
		Message: "",
		Data:    data,
	})
}
