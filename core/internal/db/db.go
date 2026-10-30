package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a database connection with single-writer guarantee
type DB struct {
	conn   *sql.DB
	mu     sync.Mutex // Serialize writes
	closed bool
}

// New creates a new database connection
func New(url string) (*DB, error) {
	url = strings.TrimPrefix(url, "sqlite:")
	path := strings.TrimPrefix(strings.Split(url, "?")[0], "file:")
	if path != "" && path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	// Parse SQLite URL (standard format: sqlite:file:/path/to/db.db)
	conn, err := sql.Open("sqlite", url)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool parameters
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := conn.Ping(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := conn.Exec("PRAGMA journal_mode = WAL"); err != nil {
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Set other important pragmas
	_, _ = conn.Exec("PRAGMA synchronous = NORMAL")
	_, _ = conn.Exec("PRAGMA cache_size = -64000") // 64 MB
	_, _ = conn.Exec("PRAGMA foreign_keys = ON")
	_, _ = conn.Exec("PRAGMA temp_store = MEMORY")
	_, _ = conn.Exec("PRAGMA busy_timeout = 5000")

	return &DB{
		conn:   conn,
		closed: false,
	}, nil
}

// Conn returns the underlying SQL connection for read operations
func (db *DB) Conn() *sql.DB {
	return db.conn
}

// QueryRow performs a read-only query that returns a single row
func (db *DB) QueryRow(query string, args ...interface{}) *sql.Row {
	return db.conn.QueryRow(query, args...)
}

// Query performs a read-only query that returns multiple rows
func (db *DB) Query(query string, args ...interface{}) (*sql.Rows, error) {
	return db.conn.Query(query, args...)
}

// Exec performs a write operation (serialized via mutex)
func (db *DB) Exec(query string, args ...interface{}) (sql.Result, error) {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.conn.Exec(query, args...)
}

// BeginTx starts a write transaction
func (db *DB) BeginTx() (*sql.Tx, error) {
	db.mu.Lock()
	tx, err := db.conn.Begin()
	if err != nil {
		db.mu.Unlock()
	}
	return tx, err
	// Note: Caller must Unlock() after defer tx.Rollback/Commit
}

// TxUnlock unlocks the write mutex after transaction
func (db *DB) TxUnlock() {
	db.mu.Unlock()
}

// QueryRowTx performs a query within a transaction (must be called after BeginTx)
func (db *DB) QueryRowTx(tx *sql.Tx, query string, args ...interface{}) *sql.Row {
	return tx.QueryRow(query, args...)
}

// ExecTx performs an exec within a transaction
func (db *DB) ExecTx(tx *sql.Tx, query string, args ...interface{}) (sql.Result, error) {
	return tx.Exec(query, args...)
}

// Backup creates a backup of the database to a file
func (db *DB) Backup(destPath string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(destPath), 0700); err != nil {
		return err
	}
	// VACUUM INTO creates a consistent snapshot including WAL contents.
	_, err := db.conn.Exec("VACUUM INTO '" + strings.ReplaceAll(destPath, "'", "''") + "'")
	if err == nil {
		err = os.Chmod(destPath, 0600)
	}
	return err
}

// Close closes the database connection
func (db *DB) Close() error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.closed {
		return nil
	}
	db.closed = true
	return db.conn.Close()
}

// Health checks database health
func (db *DB) Health() error {
	return db.conn.Ping()
}

// Statistics returns database statistics
func (db *DB) Statistics() map[string]interface{} {
	return map[string]interface{}{
		"connections": db.conn.Stats().OpenConnections,
		"in_use":      db.conn.Stats().InUse,
		"idle":        db.conn.Stats().Idle,
	}
}
