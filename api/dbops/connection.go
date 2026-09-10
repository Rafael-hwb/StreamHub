package dbops

import (
	"database/sql"

	"github.com/Rafael-hwb/streamhub/internal/config"
	"github.com/Rafael-hwb/streamhub/internal/dbconn"
)

var dbConnection *sql.DB

func Init(cfg config.Config) error{
	db, err := dbconn.Open(cfg.MySQL)
	if err != nil {
		return err
	}
	dbConnection = db
	return nil
}