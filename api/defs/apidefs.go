package defs

// requests
type UserCredential struct {
	UserName string `json:"user_name"`
	Pwd      string `json:"pwd"`
}

type VideoCreateRequest struct {
	Title string `json:"title"`
}

type VideoInfo struct {
	Id           string `json:"id"`
	AuthorId     int    `json:"author_id"`
	Title        string `json:"title"`
	DisplayCtime string `json:"display_ctime"`
}

type Comment struct {
	Id         string `json:"id"`
	VideoId    string `json:"video_id"`
	AuthorName string `json:"author_name"`
	Content    string `json:"content"`
}

type SimpleSession struct {
	UserName string `json:"user_name"`
	TTL      int64  `json:"ttl"`
}

type SignUp struct {
	Success   bool   `json:"success"`
	SessionId string `json:"session_id"`
}

type VideoDetail struct {
	Id           string `json:"id"`
	AuthorId     int    `json:"author_id"`
	AuthorName   string `json:"author_name"`
	Title        string `json:"title"`
	DisplayCtime string `json:"display_ctime"`
}

type CommentCreateRequest struct {
	Content string `json:"content"`
}
