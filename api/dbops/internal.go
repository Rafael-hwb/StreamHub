package dbops

import (
	"sync"

	"github.com/Rafael-hwb/streamhub/api/defs"
)

func (s *Store) InsertSession(sid string, TTL int64, username string) error {
	stmtIn, err := s.db.Prepare("INSERT INTO sessions (session_id, TTL, login_name) VALUES (?,?,?)")
	if err != nil {
		return err
	}
	defer stmtIn.Close()

	_, err = stmtIn.Exec(sid, TTL, username)
	if err != nil {
		return err
	}

	return nil
}

func (s *Store) RetrieveSession(sid string) (*defs.SimpleSession, error) {
	result := &defs.SimpleSession{}
	stmtOut, err := s.db.Prepare("SELECT login_name, TTL FROM sessions WHERE session_id=?")
	if err != nil {
		return nil, err
	}
	defer stmtOut.Close()

	var (
		username string
		ttl      int64
	)
	err = stmtOut.QueryRow(sid).Scan(&username, &ttl)
	if err != nil {
		return nil, err
	}

	result.UserName = username
	result.TTL = ttl
	return result, nil
}

func (s *Store) RetrieveAllSessions() (*sync.Map, error) {
	result := &sync.Map{}
	stmtOut, err := s.db.Prepare("SELECT session_id, TTL, login_name FROM sessions")
	if err != nil {
		return nil, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var id, loginName string
	var ttl int64
	for rows.Next() {
		if err = rows.Scan(&id, &ttl, &loginName); err != nil {
			return nil, err
		}
		perSimpleSession := &defs.SimpleSession{UserName: loginName, TTL: ttl}
		result.Store(id, perSimpleSession)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) DeleteSession(sid string) error {
	stmtDel, err := s.db.Prepare("DELETE FROM sessions WHERE session_id=?")
	if err != nil {
		return err
	}
	defer stmtDel.Close()

	if _, err = stmtDel.Exec(sid); err != nil {
		return err
	}
	return nil

}
