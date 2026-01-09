package database

import (
	"database/sql"
	"fmt"
	"log"
	"pg-backend/internal/config"

	_ "github.com/lib/pq"
)

var DB *sql.DB

func ConnectDB(cfg *config.Config) error {
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		cfg.DBHost, cfg.DBPort, cfg.DBUser, cfg.DBPassword, cfg.DBName)

	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return err
	}

	err = db.Ping()
	if err != nil {
		return err
	}

	DB = db
	log.Println("Connected to PostgreSQL database")
	return nil
}
