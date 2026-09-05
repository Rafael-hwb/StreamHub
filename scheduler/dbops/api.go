package dbops

import (
	"log"
	)



func AddVideoDeletionRecord(vid string) error{
	stmtIn, err := dbConnection.Prepare("INSERT INTO video_del_rec (video_id) VALUES (?)")

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