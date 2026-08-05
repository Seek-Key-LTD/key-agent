// Copyright 2026 Google LLC
// Package oracle_mem implements Unified Collective Context ("集体潜意识")
// powered by Oracle Autonomous Database 26ai (5号湖泊 Lake 5) for ADK Multi-Agent Cluster.

package oracle_mem

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	_ "github.com/sijms/go-ora/v2"
)

// VaultLake5Config represents Vault credentials for Oracle ADB 26ai (5号湖泊)
type VaultLake5Config struct {
	Host        string `json:"host"`
	Port        string `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	ServiceName string `json:"service_name"`
	TNSName     string `json:"tns_name"`
}

// CollectiveMemoryRecord represents a shared memory entry in the collective consciousness
type CollectiveMemoryRecord struct {
	ID        int64     `json:"id"`
	AgentID   string    `json:"agent_id"`
	SeatName  string    `json:"seat_name"`
	Category  string    `json:"category"` // "CollectiveConsciousness", "PublicKnowledge", "WorkflowReceipt"
	Content   string    `json:"content"`
	Tags      string    `json:"tags"`
	CreatedAt time.Time `json:"created_at"`
}

// UnifiedContextManager handles reads/writes to Oracle Lake 5
type UnifiedContextManager struct {
	vaultAddr  string
	vaultToken string
	db         *sql.DB
}

// NewUnifiedContextManager initializes connection to Oracle ADB 26ai Lake 5 via Vault
func NewUnifiedContextManager(vaultAddr, vaultToken string) (*UnifiedContextManager, error) {
	mgr := &UnifiedContextManager{
		vaultAddr:  vaultAddr,
		vaultToken: vaultToken,
	}

	cfg, err := mgr.fetchVaultConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Oracle Lake 5 config from Vault: %w", err)
	}

	// Build Oracle TCPS Connection DSN using pure-go go-ora driver
	dsn := fmt.Sprintf("oracle://%s:%s@%s:%s/%s",
		cfg.Username, cfg.Password, cfg.Host, cfg.Port, cfg.ServiceName)

	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open Oracle 26ai connection: %w", err)
	}

	mgr.db = db
	return mgr, nil
}

// fetchVaultConfig reads secret/data/oracle/config/lake5 from Vault
func (m *UnifiedContextManager) fetchVaultConfig() (*VaultLake5Config, error) {
	url := fmt.Sprintf("%s/v1/secret/data/oracle/config/lake5", m.vaultAddr)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("X-Vault-Token", m.vaultToken)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var vaultResp struct {
		Data struct {
			Data VaultLake5Config `json:"data"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &vaultResp); err != nil {
		return nil, err
	}

	return &vaultResp.Data.Data, nil
}

// EnsureTableExists creates the collective memory table in Oracle ADB 26ai
func (m *UnifiedContextManager) EnsureTableExists(ctx context.Context) error {
	query := `
	DECLARE
		e_table_exists EXCEPTION;
		PRAGMA EXCEPTION_INIT(e_table_exists, -955);
	BEGIN
		EXECUTE IMMEDIATE 'CREATE TABLE unified_collective_context (
			id NUMBER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
			agent_id VARCHAR2(100),
			seat_name VARCHAR2(100),
			category VARCHAR2(100),
			content VARCHAR2(4000),
			tags VARCHAR2(500),
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)';
	EXCEPTION
		WHEN e_table_exists THEN NULL;
	END;
	`
	_, err := m.db.ExecContext(ctx, query)
	return err
}

// RecordCollectiveMemory writes a shared collective memory entry to Oracle 26ai Lake 5
func (m *UnifiedContextManager) RecordCollectiveMemory(ctx context.Context, agentID, seatName, category, content, tags string) error {
	query := `INSERT INTO unified_collective_context (agent_id, seat_name, category, content, tags) VALUES (:1, :2, :3, :4, :5)`
	_, err := m.db.ExecContext(ctx, query, agentID, seatName, category, content, tags)
	return err
}

// RecallCollectiveMemory queries shared collective consciousness entries across subagents
func (m *UnifiedContextManager) RecallCollectiveMemory(ctx context.Context, keyword string, limit int) ([]CollectiveMemoryRecord, error) {
	query := `SELECT id, agent_id, seat_name, category, content, NVL(tags, ''), created_at 
	          FROM unified_collective_context 
	          WHERE content LIKE :1 OR tags LIKE :2 
	          ORDER BY id DESC FETCH FIRST :3 ROWS ONLY`

	searchPattern := "%" + keyword + "%"
	rows, err := m.db.QueryContext(ctx, query, searchPattern, searchPattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []CollectiveMemoryRecord
	for rows.Next() {
		var rec CollectiveMemoryRecord
		if err := rows.Scan(&rec.ID, &rec.AgentID, &rec.SeatName, &rec.Category, &rec.Content, &rec.Tags, &rec.CreatedAt); err != nil {
			return nil, err
		}
		records = append(records, rec)
	}

	return records, nil
}
