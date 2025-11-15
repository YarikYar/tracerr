package database

import (
	"fmt"
	"log"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Config holds database configuration
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
}

// DB wraps the database connection
type DB struct {
	*sqlx.DB
}

// NewDB creates a new database connection
func NewDB(config Config) (*DB, error) {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host,
		config.Port,
		config.User,
		config.Password,
		config.DBName,
		config.SSLMode,
	)

	db, err := sqlx.Connect("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Println("Successfully connected to PostgreSQL database")

	return &DB{db}, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}

// InitSchema initializes the database schema
func (db *DB) InitSchema() error {
	schema := `
	-- Trace requests table
	CREATE TABLE IF NOT EXISTS trace_requests (
		id BIGSERIAL PRIMARY KEY,
		address VARCHAR(255) NOT NULL,
		hash VARCHAR(128),
		lt BIGINT,
		scan_depth INTEGER NOT NULL DEFAULT 100,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
		updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_trace_requests_address ON trace_requests(address);
	CREATE INDEX IF NOT EXISTS idx_trace_requests_cache_key
		ON trace_requests(address, COALESCE(hash, ''), COALESCE(lt, 0), scan_depth);

	-- Trace results table
	CREATE TABLE IF NOT EXISTS trace_results (
		id BIGSERIAL PRIMARY KEY,
		request_id BIGINT NOT NULL REFERENCES trace_requests(id) ON DELETE CASCADE,
		success BOOLEAN NOT NULL DEFAULT true,
		error_message TEXT,
		original_sender VARCHAR(255) NOT NULL,
		total_transactions INTEGER NOT NULL DEFAULT 0,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_trace_results_request_id ON trace_results(request_id);

	-- Account results table
	CREATE TABLE IF NOT EXISTS account_results (
		id BIGSERIAL PRIMARY KEY,
		trace_result_id BIGINT NOT NULL REFERENCES trace_results(id) ON DELETE CASCADE,
		address VARCHAR(255) NOT NULL,
		balance_change_ton VARCHAR(50) NOT NULL,
		network_fees_ton VARCHAR(50) NOT NULL,
		created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
	);

	CREATE INDEX IF NOT EXISTS idx_account_results_trace_result_id
		ON account_results(trace_result_id);
	CREATE INDEX IF NOT EXISTS idx_account_results_address ON account_results(address);
	`

	_, err := db.Exec(schema)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	log.Println("Database schema initialized successfully")
	return nil
}
