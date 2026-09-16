package errs

type AppError struct{
	HTTPStatus int
	Code string
	Message string
	Cause error
}

func (e *AppError) Error() string{
	return e.Message
}

func BadRequest(msg string) *AppError{
	return &AppError{400, "BAD_REQUEST", msg, nil}
}

func Unauthorized(msg string) *AppError { 
	return &AppError{401, "UNAUTHORIZED", msg, nil} 
}

func Forbidden(msg string) *AppError    { 
	return &AppError{403, "FORBIDDEN", msg, nil} 
}

func NotFound(msg string) *AppError     { 
	return &AppError{404, "NOT_FOUND", msg, nil} 
}

func Internal(cause error) *AppError    { 
	return &AppError{500, "INTERNAL", "Internal server error", cause} 
}

func TooManyRequests(msg string) *AppError {
	return &AppError{429, "TOO_MANY_REQUESTS", msg, nil}
}