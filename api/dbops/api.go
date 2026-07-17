package dbops

import (
	"time"
	"api/defs"
	"api/utils"
	"database/sql"
	_ "database/sql"
	"log"
	_ "github.com/go-sql-driver/mysql"
)

func AddCredential(loginName string, pwd string) error{
	stmtIns, err := dbConnection.Prepare("INSERT INTO users (login_name,pwd) VALUES (?,?)")
	if err != nil{
		return err
	}
	_, err = stmtIns.Exec(loginName,pwd)
	if err != nil{
		return err
	}
	defer stmtIns.Close()
	return nil
}

func GetCredential(loginName string) (string, error){
	stmtOut, err := dbConnection.Prepare("SELECT pwd FROM users WHERE login_name = ?")
	if err != nil {
		log.Printf("%s", err)
		return "", err
	}
	var pwd string
	err = stmtOut.QueryRow(loginName).Scan(&pwd)
	if err != nil && err != sql.ErrNoRows{
		return "", err
	}
	defer stmtOut.Close()
	return pwd,nil
}

func DeleteCredential(loginName string, pwd string) error{
	stmtDel, err := dbConnection.Prepare("DELETE FROM users WHERE login_name = ? AND pwd = ?")
	if err != nil {
		log.Printf("DeleteUser error: %s", err)
		return err
	}
	_,err = stmtDel.Exec(loginName, pwd)
	if err != nil {
		return err
	}
	defer stmtDel.Close()
	return nil
}

func AddVideo(aid int, title string) (*defs.VideoInfo, error){
	vid, err := utils.NewUUID()
	if err != nil {
		return nil, err
	}
	t := time.Now()
	ctime := t.Format("jan 2 2006, 15:04:05")
	stmtIn, err := dbConnection.Prepare(`INSERT INTO video_info (id, title, author_id, display_ctime)
						VALUES(?, ?, ?, ?)`)
	if err != nil {
		return nil, err
	}

	_, err = stmtIn.Exec(vid, title, aid, ctime)
	if err != nil {
		return nil, err
	}

	result := &defs.VideoInfo{Id: vid, AuthorId: aid, Title: title, DisplayCtime: ctime}
	
	defer stmtIn.Close()
	return result,nil
}

func GetVideo(vid string) (*defs.VideoInfo, error){
	stmtOut, err := dbConnection.Prepare("SELECT author_id, title, display_ctime FROM video_info WHERE id=?")
	if err != nil {
		return nil, err
	}
	var(
		aid int
		title string
		ctime string
	)
	err = stmtOut.QueryRow(vid).Scan(&aid, &title, &ctime)
	if err == sql.ErrNoRows{
		return nil,nil
	}
	if err != nil {
		return nil, err
	}

	videoInfo := &defs.VideoInfo{Id: vid, AuthorId: aid, Title: title, DisplayCtime: ctime}
	defer stmtOut.Close()
	return videoInfo, nil
}

func DeleteVideo(id string) error{
	stmtDel, err := dbConnection.Prepare("DELETE FROM video_info WHERE id=?")
	if err != nil {
		return err
	}
	_, err = stmtDel.Exec(id)
	if err != nil {
		return err
	}
	
	defer stmtDel.Close()
	return nil
}