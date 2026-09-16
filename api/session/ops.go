package session

import (
	"sync"
	"time"

	"github.com/Rafael-hwb/streamhub/api/defs"
	"github.com/Rafael-hwb/streamhub/api/utils"
)

// Store 是 session 持久化所需的最小接口，*dbops.Store 自动满足它。
type Store interface {
	InsertSession(sid string, ttl int64, username string) error
	RetrieveAllSessions() (*sync.Map, error)
	DeleteSession(sid string) error
}

var sessionMap *sync.Map

func init() {
	sessionMap = &sync.Map{}
}

func DeleteSession(store Store, sid string) {
	sessionMap.Delete(sid)
	_ = store.DeleteSession(sid)
}

func LoadSessionsFromDB(store Store) error {
	r, err := store.RetrieveAllSessions()
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

func GenerateSessionId(store Store, username string) (string, error) {
	sid, err := utils.NewUUID()
	if err != nil {
		return "", err
	}

	createTime := time.Now().UnixMilli()

	TTL := createTime + 30*60*1000
	perSimpleSession := &defs.SimpleSession{UserName: username, TTL: TTL}
	sessionMap.Store(sid, perSimpleSession)
	if err := store.InsertSession(sid, TTL, username); err != nil {
		return "", err
	}
	return sid, nil
}

func IsSessionValid(store Store, sid string) (string, bool) {
	perSimpleSession, ok := sessionMap.Load(sid)
	if ok {
		nowTime := time.Now().UnixMilli()
		if nowTime < perSimpleSession.(*defs.SimpleSession).TTL {
			return perSimpleSession.(*defs.SimpleSession).UserName, true
		}
		DeleteSession(store, sid)
		return "", false
	}
	return "", false

}
