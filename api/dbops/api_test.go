package dbops

import (
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/Rafael-hwb/streamhub/internal/config"
	"github.com/Rafael-hwb/streamhub/internal/dbconn"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
)

var testStore *Store

func clearTables() error {
	tables := []string{"comments", "video_info", "sessions", "users"}

	for _, table := range tables {
		if _, err := testStore.db.Exec("TRUNCATE " + table); err != nil {
			return fmt.Errorf("truncate %s: %w", table, err)
		}
	}

	return nil
}

func TestMain(m *testing.M) {
	if os.Getenv("STREAMHUB_INTEGRATION_TEST") != "1" {
		fmt.Println("跳过 MySQL 集成测试")
		os.Exit(0)
	}

	_ = godotenv.Load("../../.env")

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("加载测试配置失败: %v\n", err)
		os.Exit(1)
	}

	if cfg.MySQL.DB != "streamhub_test" {
		fmt.Printf("拒绝清空非测试数据库: %s\n", cfg.MySQL.DB)
		os.Exit(1)
	}

	db, err := dbconn.Open(cfg.MySQL)
	if err != nil {
		fmt.Printf("连接测试数据库失败: %v\n", err)
		os.Exit(1)
	}
	testStore = NewStore(db)

	if err := clearTables(); err != nil {
		fmt.Printf("测试前清表失败: %v\n", err)
		os.Exit(1)
	}

	code := m.Run()

	if err := clearTables(); err != nil {
		fmt.Printf("测试后清表失败: %v\n", err)
		code = 1
	}

	os.Exit(code)
}

// test:USers
func TestUserWorkflow(t *testing.T) {
	clearTables()
	t.Run("Add", testAddUser)
	t.Run("Get", testGetUser)
	t.Run("Delete", testDeleteUser)
	t.Run("Reget", testRegetUser)
}

func testAddUser(t *testing.T) {
	err := testStore.AddCredential("avenssi", "123")
	if err != nil {
		t.Errorf("Error of AddUser: %v", err)
	}
}

func testGetUser(t *testing.T) {
	pwdHash, err := testStore.GetPasswordHash("avenssi")
	if err != nil {
		t.Errorf("GetCredential: %v", err)
		return
	}
	if pwdHash == "" {
		t.Errorf("expected a stored hash, got empty")
		return
	}
	if pwdHash == "123" {
		t.Errorf("password must not be stored in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(pwdHash), []byte("123")); err != nil {
		t.Errorf("password should match stored hash: %v", err)
	}
}

func testDeleteUser(t *testing.T) {
	err := testStore.DeleteCredential("avenssi")
	if err != nil {
		t.Errorf("Error of DeleteUser: %v", err)
	}
}

func testRegetUser(t *testing.T) {
	pwd, err := testStore.GetPasswordHash("avenssi")
	if err != nil {
		t.Errorf("Error of GetUser: %v", err)
	}
	if pwd != "" {
		t.Errorf("DeleteUser test failed")
	}
}

//test:vedio
var tempvid string

func TestVideoWorkflow(t *testing.T) {
	clearTables()
	t.Run("AddVideo", testAddVideo)
	t.Run("GetVideo", testGetVideo)
	t.Run("DeleteVideo", testDeleteVideo)
	t.Run("RegetVideo", testRegetVideo)
}

func testAddVideo(t *testing.T) {
	video, err := testStore.AddVideo(1, "video1")
	if err != nil {
		t.Errorf("Error of AddVideo: %v", err)
	}
	tempvid = video.Id
}

func testGetVideo(t *testing.T) {
	video, err := testStore.GetVideo(tempvid)
	if err != nil || video.Title != "video1" {
		t.Errorf("Error of GetVideo: %v", err)
	}
}

func testDeleteVideo(t *testing.T) {
	err := testStore.DeleteVideo(tempvid)
	if err != nil {
		t.Errorf("Error of DeleteVideo: %v", err)
	}
}

func testRegetVideo(t *testing.T) {
	video, err := testStore.GetVideo(tempvid)
	if err != nil {
		t.Errorf("Error of Regetvideo: %v", err)
	}
	if video != nil {
		t.Error("Delete video failed.")
	}
}

//test:comment
func TestCommentWorkflow(t *testing.T) {
	clearTables()
	t.Run("Add User", testAddUser)
	t.Run("Add Comment", testAddComment)
	t.Run("List Comments", testListcomments)

}

func testAddComment(t *testing.T) {
	vid := "12345"
	aid := 1
	content1 := "Add the first comment."
	content2 := "Add the second comment."
	err1 := testStore.AddComment(vid, aid, content1)
	if err1 != nil {
		t.Errorf("Error of add the fitst comment: %v", err1)
	}

	err2 := testStore.AddComment(vid, aid, content2)
	if err2 != nil {
		t.Errorf("Error of add the second comment: %v", err2)
	}
}

func testListcomments(t *testing.T) {
	vid := "12345"
	originTime := 1514764880
	endTime, _ := strconv.Atoi(strconv.FormatInt(time.Now().UnixNano()/1000000000, 10))

	res, err := testStore.ListComments(vid, originTime, endTime)
	if err != nil {
		t.Errorf("Error of ListComments: %v", err)
	}

	for i, comment := range res {
		t.Logf("comment%d: %v\n", i, comment)
	}

}
