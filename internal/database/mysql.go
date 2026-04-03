package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

// Config represents MySQL connection configuration
type Config struct {
	Host     string
	Port     string
	User     string
	Password string
	Database string
	Params   map[string]string
}

// NewConfigFromEnv creates a MySQL config from environment variables
func NewConfigFromEnv() *Config {
	return &Config{
		Host:     os.Getenv("MYSQL_HOST"),
		Port:     os.Getenv("MYSQL_PORT"),
		User:     os.Getenv("MYSQL_USER"),
		Password: os.Getenv("MYSQL_PASSWORD"),
		Database: os.Getenv("MYSQL_DATABASE"),
		Params: map[string]string{
			"parseTime": "true",
			"charset":   "utf8mb4",
			"collation": "utf8mb4_unicode_ci",
		},
	}
}

// DSN returns the Data Source Name for MySQL connection
func (c *Config) DSN() string {
	config := mysql.Config{
		User:   c.User,
		Passwd: c.Password,
		Net:    "tcp",
		Addr:   fmt.Sprintf("%s:%s", c.Host, c.Port),
		DBName: c.Database,
		Params: c.Params,
	}
	
	return config.FormatDSN()
}

// Connect establishes a MySQL database connection with connection pooling
func Connect(config *Config) (*sql.DB, error) {
	if config == nil {
		config = NewConfigFromEnv()
	}

	dsn := config.DSN()
	
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)
	db.SetConnMaxIdleTime(2 * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping MySQL: %w", err)
	}

	log.Printf("Successfully connected to MySQL database: %s", config.Database) //nolint:G706 // database name from server config, not user input
	return db, nil
}

// Close safely closes the database connection
func Close(db *sql.DB) {
	if db != nil {
		db.Close()
		log.Println("MySQL connection closed")
	}
}

// Query executes a query and returns rows
func Query(db *sql.DB, query string, args ...interface{}) (*sql.Rows, error) {
	return db.Query(query, args...)
}

// QueryRow executes a query that returns at most one row
func QueryRow(db *sql.DB, query string, args ...interface{}) *sql.Row {
	return db.QueryRow(query, args...)
}

// Exec executes a query without returning rows
func Exec(db *sql.DB, query string, args ...interface{}) (sql.Result, error) {
	return db.Exec(query, args...)
}