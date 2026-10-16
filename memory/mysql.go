// Copyright 2026 Google LLC
//
// Pure Go MySQL Memory Store for KeyAgent Guest (客卿) Mode
// Optimized for lightweight VM/container execution, zero TUN/HCP dependencies.
// Backed by River1 MySQL (26.7.0) via PVE HAProxy Ingress or direct endpoint.

package memory

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// MySQLConfig holds connection parameters for River1 MySQL
type MySQLConfig struct {
	Host     string // e.g. "192.168.31.77" (HAProxy VIP) or direct IP
	Port     int    // e.g. 33061 (River1 HAProxy port) or 3306
	Database string // "keyagent"
	User     string // e.g. "keyagent_app" or "root"
	Password string
	DSN      string // raw DSN override (e.g. from MYSQL_DSN env)
}

// MySQLMemoryStore provides persistent memory storage for Guest Agent
type MySQLMemoryStore struct {
	db *sql.DB
}

// NewMySQLMemoryStore connects to MySQL (River1) using standard pure Go driver
func NewMySQLMemoryStore(cfg MySQLConfig) (*MySQLMemoryStore, error) {
	dsn := cfg.DSN
	if dsn == "" {
		dsn = fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=true&loc=Local&timeout=5s",
			cfg.User, cfg.Password, cfg.Host, cfg.Port, cfg.Database)
	}

	log.Printf("[Guest Memory] Connecting to MySQL (River1) for Guest Agent (%s:%d/%s)...", cfg.Host, cfg.Port, cfg.Database)

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("mysql open error: %w", err)
	}

	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	// Quick ping with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Printf("[Guest Memory] Notice: Initial ping unreached (will retry on demand): %v", err)
	} else {
		log.Printf("[Guest Memory] ✅ Successfully connected to MySQL (River1)")
		// Auto ensure schema
		initQuery := `
		CREATE TABLE IF NOT EXISTS agent_memory (
			id VARCHAR(64) PRIMARY KEY,
			agent_id VARCHAR(64) NOT NULL,
			memory_text TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			INDEX idx_agent_created (agent_id, created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;`
		if _, err := db.ExecContext(ctx, initQuery); err != nil {
			log.Printf("[Guest Memory] Auto table schema notice: %v", err)
		}
	}

	return &MySQLMemoryStore{db: db}, nil
}

// SaveMemory stores an Agent memory record in MySQL
func (s *MySQLMemoryStore) SaveMemory(ctx context.Context, rec MemoryRecord) error {
	log.Printf("[Guest Memory] Storing Memory for Agent [%s]: '%s'", rec.AgentID, rec.MemoryText)
	if s.db == nil {
		return nil
	}

	query := `INSERT INTO agent_memory (id, agent_id, memory_text, created_at) 
	          VALUES (?, ?, ?, ?) 
	          ON DUPLICATE KEY UPDATE memory_text = VALUES(memory_text), created_at = VALUES(created_at)`
	createdAt := rec.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}

	_, err := s.db.ExecContext(ctx, query, rec.ID, rec.AgentID, rec.MemoryText, createdAt)
	if err != nil {
		log.Printf("[Guest Memory] DB exec error: %v", err)
		return err
	}
	return nil
}

// QueryRecentMemories queries the latest N memories for an agent
func (s *MySQLMemoryStore) QueryRecentMemories(ctx context.Context, agentID string, limit int) ([]MemoryRecord, error) {
	if s.db == nil {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}

	query := `SELECT id, agent_id, memory_text, created_at 
	          FROM agent_memory 
	          WHERE agent_id = ? 
	          ORDER BY created_at DESC 
	          LIMIT ?`

	rows, err := s.db.QueryContext(ctx, query, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []MemoryRecord
	for rows.Next() {
		var rec MemoryRecord
		if err := rows.Scan(&rec.ID, &rec.AgentID, &rec.MemoryText, &rec.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, rec)
	}
	return results, nil
}

// Close closes DB connection
func (s *MySQLMemoryStore) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
