package dbops

import (
	"database/sql"
	"os"
	"fmt"
	"path/filepath"
	_"github.com/go-sql-driver/mysql"
	"github.com/joho/godotenv"
)

var(
	dbConnection *sql.DB
	err error
)

func init(){
	cwd, _ := os.Getwd()
	envPath := filepath.Join(cwd, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envPath = filepath.Join(cwd, "..", ".env")
	}
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envPath = filepath.Join(cwd, "api", ".env")
	}
	if _, err := os.Stat(envPath); !os.IsNotExist(err) {
		if err := godotenv.Load(envPath); err != nil {
			fmt.Printf("Warning: failed to load .env file: %v\n", err)
		}
	}

	user := os.Getenv("MYSQL_USER")
	if user == "" {
		user = "root"
	}
	pwd := os.Getenv("MYSQL_PWD")
	host := os.Getenv("MYSQL_HOST")
	if host == "" {
		host = "localhost"
	}
	port := os.Getenv("MYSQL_PORT")
	if port == "" {
		port = "3306"
	}
	db := os.Getenv("MYSQL_DB")
	if db == "" {
		db = "streamhub"
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, pwd, host, port, db)
	dbConnection, err = sql.Open("mysql", dsn)
	if err != nil{
		panic(err.Error())
	}
}
