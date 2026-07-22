package dbops

import (
	"github.com/Rafael-hwb/streamhub/api/defs"
	"strconv"
	"sync"
)

func InsertSession(sid string, TTL int64, username string) error{
	stringTTL := strconv.FormatInt(TTL, 10)
	stmtIn, err := dbConnection.Prepare("INSERT INTO sessions (session_id, TTL, login_name) VALUES (?,?,?)")
	if err != nil {
		return err
	}
	defer stmtIn.Close()

	_, err = stmtIn.Exec(sid, stringTTL, username)
	if err != nil {
		return err
	}
	
	return nil
}

func RetrieveSession(sid string) (*defs.SimpleSession, error){
	result := &defs.SimpleSession{}
	stmtOut ,err := dbConnection.Prepare("SELECT user_name, TTL FROM sessions WHERE session_id=?")
	if err != nil {
		return nil, err
	}
	defer stmtOut.Close()

	var username, stringTTL string
	
	err = stmtOut.QueryRow(sid).Scan(&username, &stringTTL)
	if err != nil {
		return nil, err
	}

	if TTL,err := strconv.ParseInt(stringTTL, 10, 64); err == nil{
		result.UserName = username
		result.TTL = TTL
	}else{
		return nil,err
	}

	return result,nil
	
}

func RetrieveAllSessions() (*sync.Map, error){
	result := &sync.Map{}
	stmtOut, err := dbConnection.Prepare("SELECT * FROM sessions")
	if err != nil {
		return nil, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query()
	if err != nil {
		return nil, err
	}
	var id, stringTTL, loginName string
	for rows.Next(){
		if err = rows.Scan(&id, &stringTTL, &loginName); err != nil{
			return nil, err
		}
		if TTL, err := strconv.ParseInt(stringTTL, 10, 64);err ==nil{
			perSimpleSession := &defs.SimpleSession{UserName: loginName, TTL: TTL}
			result.Store(id, perSimpleSession)
		}else{
			return nil, err
		}
	}
	return result,nil
}



func DeleteSession(sid string) error{
	stmtDel, err := dbConnection.Prepare("DELETE FROM sessions WHERE session_id=?")
	if err != nil {
		return err
	}
	defer stmtDel.Close()

	if _, err = stmtDel.Exec(sid); err != nil{
		return err
	}
	return nil

}