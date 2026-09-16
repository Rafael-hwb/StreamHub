package dbops

import (
	"database/sql"
	"log"
	"time"

	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/utils"
	_ "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

// 用户信息部分
func AddCredential(loginName string, pwd string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	stmtIns, err := dbConnection.Prepare("INSERT INTO users (login_name,pwd) VALUES (?,?)")
	if err != nil {
		return err
	}
	defer stmtIns.Close()

	_, err = stmtIns.Exec(loginName, string(hash))
	if err != nil {
		return err
	}

	return nil
}

func GetPasswordHash(loginName string) (string, error) {
	stmtOut, err := dbConnection.Prepare("SELECT pwd FROM users WHERE login_name = ?")
	if err != nil {
		log.Printf("%s", err)
		return "", err
	}
	defer stmtOut.Close()

	var pwdHash string
	err = stmtOut.QueryRow(loginName).Scan(&pwdHash)
	if err != nil && err != sql.ErrNoRows {
		return "", err
	}

	return pwdHash, nil
}

func DeleteCredential(loginName string) error {
	stmtDel, err := dbConnection.Prepare("DELETE FROM users WHERE login_name = ?")
	if err != nil {
		log.Printf("DeleteUser error: %s", err)
		return err
	}
	defer stmtDel.Close()

	_, err = stmtDel.Exec(loginName)
	if err != nil {
		return err
	}

	return nil
}

// 视频信息部分
func AddVideo(aid int, title string) (*defs.VideoInfo, error) {
	vid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	t := time.Now()
	ctime := t.Format("Jan 2 2006, 15:04:05")

	stmtIn, err := dbConnection.Prepare(`INSERT INTO video_info (id, title, author_id, display_ctime)
						VALUES(?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}
	defer stmtIn.Close()

	_, err = stmtIn.Exec(vid, title, aid, ctime)
	if err != nil {
		return nil, err
	}

	result := &defs.VideoInfo{Id: vid, AuthorId: aid, Title: title, DisplayCtime: ctime}
	return result, nil
}

func GetVideo(vid string) (*defs.VideoInfo, error) {
	stmtOut, err := dbConnection.Prepare("SELECT author_id, title, display_ctime FROM video_info WHERE id=?")
	if err != nil {
		return nil, err
	}
	defer stmtOut.Close()

	var (
		aid   int
		title string
		ctime string
	)
	err = stmtOut.QueryRow(vid).Scan(&aid, &title, &ctime)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	videoInfo := &defs.VideoInfo{Id: vid, AuthorId: aid, Title: title, DisplayCtime: ctime}
	return videoInfo, nil
}

func DeleteVideo(id string) error {
	stmtDel, err := dbConnection.Prepare("DELETE FROM video_info WHERE id=?")
	if err != nil {
		return err
	}
	defer stmtDel.Close()

	_, err = stmtDel.Exec(id)
	if err != nil {
		return err
	}

	return nil
}

// 评论信息部分
func AddComment(vid string, aid int, content string) error {
	id, err := utils.NewUUID()
	if err != nil {
		return err
	}

	stmtIn, err := dbConnection.Prepare("INSERT INTO comments (id, video_id, author_id, content, create_time) VALUES (?, ?, ?, ?, NOW())")
	if err != nil {
		return err
	}
	defer stmtIn.Close()

	_, err = stmtIn.Exec(id, vid, aid, content)
	if err != nil {
		return err
	}

	return nil
}

func ListComments(vid string, originTime int, endTime int) ([]*defs.Comment, error) {
	stmtOut, err := dbConnection.Prepare(`SELECT comments.id, users.login_name, comments.content
										FROM comments
										INNER JOIN users ON comments.author_id = users.id
										where comments.video_id = ? 
										AND comments.create_time > FROM_UNIXTIME(?) 
										AND comments.create_time <= FROM_UNIXTIME(?)`)
	if err != nil {
		return nil, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query(vid, originTime, endTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*defs.Comment

	for rows.Next() {
		var id, name, content string
		if err := rows.Scan(&id, &name, &content); err != nil {
			return result, err
		}
		result = append(result, &defs.Comment{VideoId: id, AuthorName: name, Content: content})
	}

	if err := rows.Err(); err != nil {
		return result, err
	}

	return result, nil
}

// 额外检索
func GetUserIDByName(loginName string) (int, error) {
	stmtOut, err := dbConnection.Prepare("SELECT id FROM users WHERE login_name=?")
	if err != nil {
		return 0, err
	}
	defer stmtOut.Close()

	var id int
	err = stmtOut.QueryRow(loginName).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	return id, nil
}

func ListVideosByAuthor(aid int) ([]*defs.VideoInfo, error) {
	var result []*defs.VideoInfo
	stmtOut, err := dbConnection.Prepare("SELECT id, title, display_ctime FROM video_info WHERE author_id = ?")

	if err != nil {
		return result, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query(aid)
	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, title, ctime string
		if err := rows.Scan(&id, &title, &ctime); err != nil {
			return result, err
		}
		videoInfo := &defs.VideoInfo{
			Id:           id,
			Title:        title,
			DisplayCtime: ctime,
			AuthorId:     aid,
		}
		result = append(result, videoInfo)
	}

	if err := rows.Err(); err != nil {
		return result, err
	}

	return result, nil
}

func GetVideoDetail(vid string) (*defs.VideoDetail, error) {
	stmtOut, err := dbConnection.Prepare(`SELECT video_info.id, video_info.author_id,
							video_info.title, video_info.display_ctime, users.login_name
						FROM video_info
						INNER JOIN users ON video_info.author_id = users.id
						WHERE video_info.id = ?`)
	if err != nil {
		return nil, err
	}
	defer stmtOut.Close()

	var (
		id         string
		aid        int
		title      string
		ctime      string
		authorName string
	)
	err = stmtOut.QueryRow(vid).Scan(&id, &aid, &title, &ctime, &authorName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	detail := &defs.VideoDetail{Id: id, AuthorId: aid, AuthorName: authorName,
		Title: title, DisplayCtime: ctime}
	return detail, nil
}

func ListCommentsByVideo(vid string) ([]*defs.Comment, error) {
	var result []*defs.Comment

	stmtOut, err := dbConnection.Prepare(`SELECT comments.id, users.login_name, comments.content
						FROM comments
						INNER JOIN users ON comments.author_id = users.id
						WHERE comments.video_id = ?`)

	if err != nil {
		return result, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query(vid)

	if err != nil {
		return result, err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, content string
		if err := rows.Scan(&id, &name, &content); err != nil {
			return result, err
		}
		result = append(result, &defs.Comment{
			Id:         id,
			AuthorName: name,
			Content:    content,
		})
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	return result, nil
}

func ListAllVideos() ([]*defs.VideoDetail, error) {
	var result []*defs.VideoDetail

	stmtOut, err := dbConnection.Prepare(`SELECT video_info.id, video_info.author_id,
						video_info.title, video_info.display_ctime, users.login_name
						FROM video_info
						INNER JOIN users ON video_info.author_id = users.id`)

	if err != nil {
		return result, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query()
	if err != nil {
		return result, err
	}

	for rows.Next() {
		var (
			id         string
			aid        int
			title      string
			ctime      string
			authorName string
		)
		if err := rows.Scan(&id, &aid, &title, &ctime, &authorName); err != nil {
			return result, err
		}
		result = append(result, &defs.VideoDetail{Id: id, AuthorId: aid, AuthorName: authorName,
			Title: title, DisplayCtime: ctime})
	}

	if err := rows.Err(); err != nil {
		return result, err
	}

	return result, nil

}
