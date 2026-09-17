package dbops

import (
	"context"
	"log"
)

func (s *Store) ReadVideoDeletionRecord(ctx context.Context, count int) ([]string, error) {
	var ids []string
	stmtOut, err := s.db.PrepareContext(ctx, "SELECT video_id FROM video_del_rec LIMIT ?")
	if err != nil {
		return ids, err
	}
	defer stmtOut.Close()

	rows, err := stmtOut.QueryContext(ctx, count)
	if err != nil {
		log.Printf("Query VideoDeletionRecord failed: %v", err)
		return ids, err
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return ids, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (s *Store) DeleteVideoDeletionRecord(ctx context.Context, vid string) error {
	stmtDel, err := s.db.PrepareContext(ctx, "DELETE FROM video_del_rec WHERE video_id =? ")
	if err != nil {
		return err
	}

	defer stmtDel.Close()

	_, err = stmtDel.ExecContext(ctx, vid)
	if err != nil {
		log.Printf("Delete VideoDeletionRecord failed: %v", err)
		return err
	}

	return nil
}
