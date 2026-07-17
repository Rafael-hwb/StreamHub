package dbops

import (
	"testing"
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

func TestUserWorkflow(t *testing.T){
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


var tempvid string

func TestVideoWorkflow(t *testing.T){
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