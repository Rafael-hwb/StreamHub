package main

import (
	"net/http"
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
)
