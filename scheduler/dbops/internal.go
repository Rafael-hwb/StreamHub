package dbops

import (
	"log"
)

func ReadVideoDeletionRecord(count int) ([]string, error) {
	var ids []string
	stmtOut, err := dbConnection.Prepare("SELECT video_id FROM video_del_rec LIMIT ?")
	if err != nil {
		return ids, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.Query(count)
	if err != nil {
		log.Printf("Query VideoDeletionRecord failed: %v", err)
		return ids, err
	}

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func DeleteVideoDeletionRecord(vid string) error {
	stmtDel, err := dbConnection.Prepare("DELETE FROM video_del_rec WHERE video_id =? ")
	if err != nil {
		return err
	}

	defer stmtDel.Close()

	_, err = stmtDel.Exec(vid)
	if err != nil {
		log.Printf("Delete VideoDeletionRecord failed: %v", err)
		return err
	}

	return nil
}
