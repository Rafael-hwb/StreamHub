package main

import (
	"net/http"
)

const(
	VIDEO_DIR = "./videos"
	MAX_UPLOAD_SIZE = 1024*1024*50 //50MB
)

type ErrStruct struct {
	ErrMessage string `json:"error_message"`
	ErrCode    string `json:"error_code"`
}
type ErrResponse struct {
	HttpCode int
	Err      ErrStruct
}

var (
	ErrorTooManyRequests = ErrResponse{
		HttpCode: http.StatusTooManyRequests,
		Err: ErrStruct{
			ErrMessage: "Too many requests.",
			ErrCode:    "001",
		},
	}

	ErrorInternalFaults = ErrResponse{
		HttpCode: http.StatusInternalServerError,
		Err: ErrStruct{
			ErrMessage: "Internal server error.",
			ErrCode:    "002",
		},
	}

		ErrorFileTooBig = ErrResponse{
		HttpCode: http.StatusBadRequest,
		Err: ErrStruct{
			ErrMessage: "File is too big.",
			ErrCode:    "002",
		},
	}
)