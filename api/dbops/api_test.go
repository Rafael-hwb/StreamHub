package dbops

import (
	"testing"
	"time"
	"strconv"
)

func clearTables(){
	dbConnection.Exec("TRUNCATE users")
	dbConnection.Exec("TRUNCATE video_info")
	dbConnection.Exec("TRUNCATE comments")
	dbConnection.Exec("TRUNCATE sessions")
}

func TestMain(m *testing.M){
	clearTables()
	m.Run()
	clearTables()
}


//test:USers
func TestUserWorkflow(t *testing.T){
	clearTables()
	t.Run("Add",testAddUser)
	t.Run("Get",testGetUser)
	t.Run("Delete",testDeleteUser)
	t.Run("Reget",testRegetUser)
}

func testAddUser(t *testing.T){
	err := AddCredential("avenssi", "123")
	if err != nil{
		t.Errorf("Error of AddUser: %v",err)
	}
}

func testGetUser(t *testing.T){
	pwd, err := GetCredential("avenssi")
	if err != nil || pwd != "123"{
		t.Errorf("Error of GetUser: %v",err)
	}
}


func testDeleteUser(t *testing.T){
	err := DeleteCredential("avenssi", "123")
	if err != nil {
		t.Errorf("Error of DeleteUser: %v",err)
	}
}

func testRegetUser(t *testing.T){
	pwd, err := GetCredential("avenssi")
	if err != nil{
		t.Errorf("Error of GetUser: %v",err)
	}
	if pwd != ""{
		t.Errorf("DeleteUser test failed")
	}
}


//test:vedio
var tempvid string

func TestVideoWorkflow(t *testing.T){
	clearTables()
	t.Run("AddVideo",testAddVideo)
	t.Run("GetVideo",testGetVideo)
	t.Run("DeleteVideo",testDeleteVideo)
	t.Run("RegetVideo",testRegetVideo)
}


func testAddVideo(t *testing.T){
	video, err := AddVideo(1, "video1")
	if err != nil{
		t.Errorf("Error of AddVideo: %v",err)
	}
	tempvid = video.Id
}

func testGetVideo(t *testing.T){
	video,err := GetVideo(tempvid)
	if err != nil || video.Title != "video1"{
		t.Errorf("Error of GetVideo: %v", err)
	}
}

func testDeleteVideo(t *testing.T){
	err := DeleteVideo(tempvid)
	if err != nil {
		t.Errorf("Error of DeleteVideo: %v", err)
	}
}

func testRegetVideo(t *testing.T){
	video,err := GetVideo(tempvid)
	if err != nil {
		t.Errorf("Error of Regetvideo: %v", err)
	}
	if video != nil {
		t.Error("Delete video failed.")
	}
}




//test:comment
func TestCommentWorkflow(t *testing.T){
	clearTables()
	t.Run("Add User", testAddUser)
	t.Run("Add Comment", testAddComment)
	t.Run("List Comments", testListcomments)

}

func testAddComment(t *testing.T){
	vid := "12345"
	aid := 1
	content1 := "Add the first comment."
	content2 := "Add the second comment."
	err1 := AddComment(vid, aid, content1)
	if err1 != nil {
		t.Errorf("Error of add the fitst comment: %v", err1)
	}
	
	err2 := AddComment(vid, aid, content2)
	if err2 != nil {
		t.Errorf("Error of add the second comment: %v", err2)
	}
}

func testListcomments(t *testing.T){
	vid := "12345"
	originTime := 1514764880
	endTime, _ := strconv.Atoi(strconv.FormatInt(time.Now().UnixNano()/1000000000, 10))

	res, err := ListComments(vid, originTime, endTime)
	if err != nil {
		t.Errorf("Error of ListComments: %v", err)
	}

	for i, comment := range(res){
		t.Logf("comment%d: %v\n", i, comment)
	}

}