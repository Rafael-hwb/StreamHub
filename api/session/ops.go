package session

import (
	"github.com/Rafael-hwb/streamhub/api/dbops"
	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/utils"
	"sync"
	"time"
)


var sessionMap *sync.Map 

func init(){
	sessionMap = &sync.Map{} 
}

func DeleteSession(sid string){
	//I think lost the error
	sessionMap.Delete(sid)
	dbops.DeleteSession(sid)
}

func LoadSessionsFromDB(){
	r, err := dbops.RetrieveAllSessions()
	if err != nil{
		return
	}
	r.Range(func(k, v interface{}) bool{
		perSimpleSession := v.(*defs.SimpleSession)
		sessionMap.Store(k, perSimpleSession)
		return true
	})
}


func GenerateSessionId(username string) string{ 
	sid, _ := utils.NewUUID()
	createTime := time.Now().UnixMilli()
	TTL := createTime + 30 * 60 * 1000
	perSimpleSession := &defs.SimpleSession{UserName: username, TTL: TTL}
	sessionMap.Store(sid, perSimpleSession)
	dbops.InsertSession(sid, TTL , username)
	return sid
}


func IsSessionValid(sid string) (string, bool){
	perSimpleSession, isValid := sessionMap.Load(sid) 
	if isValid {
		nowTime := time.Now().UnixMilli()
		if nowTime < perSimpleSession.(*defs.SimpleSession).TTL{
			return perSimpleSession.(*defs.SimpleSession).UserName, true
		}
	}else{
		DeleteSession(sid)
		return "",false
	}
	return "",false
}

