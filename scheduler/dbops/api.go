package dbops

import (
	"log"
)

func (s *Store) AddVideoDeletionRecord(vid string) error {
	stmtIn, err := s.db.Prepare("INSERT INTO video_del_rec (video_id) VALUES (?)")

	if err != nil {
		return err
	}

	defer stmtIn.Close()

	_, err = stmtIn.Exec(vid)
	if err != nil {
		log.Printf("AddVideoDeletionRecord error: %v", err)
		return err
	}

	return nil
}
