package dbconn

import (
	"database/sql"
	"fmt"
	"time"
	"github.com/Rafael-hwb/streamhub/internal/config"
)

func Open(m config.MySQL) (*sql.DB, error){
	db, err := sql.Open("mysql", m.DSN())

	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}

	if err := db.Ping(); err != nil{
		return nil, fmt.Errorf("ping mysql: %w", err)
	}

	db.SetMaxOpenConns(20)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	return db, nil
}