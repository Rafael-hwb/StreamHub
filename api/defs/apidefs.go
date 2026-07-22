package defs

//requests
type UserCredential struct{
 UserName string `json:"user_name"`
 Pwd string `json:"pwd"`
}

type VideoInfo struct{
	Id string
	AuthorId int
	Title string
	DisplayCtime string
}

type Comment struct{
	Id string
	VideoId string
	AuthorName string
	Content string 
}


type SimpleSession struct{
	UserName string
	TTL int64
}

type SignUp struct{
	Success bool `json:"success"`
	SessionId string `json:"session_id"`
}