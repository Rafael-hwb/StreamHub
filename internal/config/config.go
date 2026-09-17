package config

import (
	"fmt"
	"os"
)

type MySQL struct {
	User string
	Pwd  string
	Host string
	Port string
	DB   string
}

func (m MySQL) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", m.User, m.Pwd, m.Host, m.Port, m.DB)
}

type Config struct {
	MySQL MySQL
	SchedulerURL string
}

func Load() (*Config, error) {
	mysql, err := loadMySQL()
	if err != nil {
		return nil, err
	}
	return &Config{
		MySQL: mysql,
		SchedulerURL: envOr("SCHEDULER_URL", "http://localhost:9001"),
		}, nil
}

func loadMySQL() (MySQL, error) {
	pwd, err := envRequired("MYSQL_PWD")
	if err != nil {
		return MySQL{}, err
	}

	return MySQL{
		User: envOr("MYSQL_USER", "root"),
		Pwd:  pwd,
		Host: envOr("MYSQL_HOST", "localhost"),
		Port: envOr("MYSQL_PORT", "3306"),
		DB:   envOr("MYSQL_DB", "streamhub"),
	}, nil
}

func envRequired(key string) (string, error) {
	v := os.Getenv(key)
	if v == "" {
		return "", fmt.Errorf("%s is required", key)
	}
	return v, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
