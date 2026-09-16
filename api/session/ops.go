package session

import (
	"github.com/Rafael-hwb/streamhub/api/dbops"
	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/utils"
	"sync"
	"time"
)

var sessionMap *sync.Map

func init() {
	sessionMap = &sync.Map{}
}

func DeleteSession(sid string) {
	//I think lost the error
	sessionMap.Delete(sid)
	dbops.DeleteSession(sid)
}

func LoadSessionsFromDB() error{
	r, err := dbops.RetrieveAllSessions()
	if err != nil {
		return err
	}
	r.Range(func(k, v interface{}) bool {
		perSimpleSession := v.(*defs.SimpleSession)
		sessionMap.Store(k, perSimpleSession)
		return true
	})
	return nil
}

func GenerateSessionId(username string) (string, error) {
	sid, err:= utils.NewUUID()
	if err != nil{
		return "", err
	}

	createTime := time.Now().UnixMilli()

	TTL := createTime + 30*60*1000
	perSimpleSession := &defs.SimpleSession{UserName: username, TTL: TTL}
	sessionMap.Store(sid, perSimpleSession)
	if err := dbops.InsertSession(sid, TTL, username);err != nil{
		return "", err
	}
	return sid, nil
}

func IsSessionValid(sid string) (string, bool) {
	perSimpleSession, ok := sessionMap.Load(sid)
	if ok {
		nowTime := time.Now().UnixMilli()
		if nowTime < perSimpleSession.(*defs.SimpleSession).TTL {
			return perSimpleSession.(*defs.SimpleSession).UserName, true
		}
			DeleteSession(sid)
			return "", false
	}
	return "", false

}