// Copyright 2026 Google LLC
//
// Agent Dispatcher, Memory Bank & Matrix Unified Communications Orchestrator

package integration

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/messaging"
)

// DispatcherService handles Gitea webhook triggers, Vault/Authentik auth, MySQL/Oracle Memory, Matrix Pager & Calendar locking
type DispatcherService struct {
	AuthentikURL string
	VaultAddr    string
	MySQLStore   *memory.MySQLMemoryStore
	OracleStore  *memory.OracleMemoryStore
	MatrixClient *messaging.MatrixClient
}

// NewDispatcherService initializes a new DispatcherService instance
func NewDispatcherService(authentikURL, vaultAddr string) *DispatcherService {
	// Initialize Pure Go MySQL Memory Store for Guest mode if configured in environment
	var mysqlStore *memory.MySQLMemoryStore
	mysqlHost := os.Getenv("MYSQL_HOST")
	mysqlDSN := os.Getenv("MYSQL_DSN")
	mysqlUser := os.Getenv("MYSQL_USER")
	mysqlPass := os.Getenv("MYSQL_PASSWORD")
	mysqlDB := os.Getenv("MYSQL_DATABASE")
	if mysqlDB == "" {
		mysqlDB = "keyagent"
	}
	mysqlPort := 33061
	if pStr := os.Getenv("MYSQL_PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil {
			mysqlPort = p
		}
	}

	if mysqlDSN != "" || (mysqlHost != "" && mysqlUser != "" && mysqlPass != "") {
		store, err := memory.NewMySQLMemoryStore(memory.MySQLConfig{
			Host:     mysqlHost,
			Port:     mysqlPort,
			Database: mysqlDB,
			User:     mysqlUser,
			Password: mysqlPass,
			DSN:      mysqlDSN,
		})
		if err != nil {
			log.Printf("[Guest Memory] ⚠️ Failed to initialize MySQL store: %v", err)
		} else {
			mysqlStore = store
		}
	} else {
		log.Printf("[Guest Memory] ⚪ MySQL memory store not configured in environment (set MYSQL_HOST/MYSQL_PASSWORD or MYSQL_DSN to enable)")
	}

	// Oracle Memory Store fallback (only if ORACLE_HOST/ORACLE_USER/ORACLE_PASSWORD are explicitly configured)
	var oracleStore *memory.OracleMemoryStore
	oracleHost := os.Getenv("ORACLE_HOST")
	oracleUser := os.Getenv("ORACLE_USER")
	oraclePass := os.Getenv("ORACLE_PASSWORD")
	oracleService := os.Getenv("ORACLE_SERVICE_NAME")
	if oracleHost != "" && oracleUser != "" && oraclePass != "" {
		oraclePort := 11522
		if pStr := os.Getenv("ORACLE_PORT"); pStr != "" {
			if p, err := strconv.Atoi(pStr); err == nil {
				oraclePort = p
			}
		}
		if oracleService == "" {
			oracleService = "g8dfe5cebce8245_lake2_medium.adb.oraclecloud.com"
		}
		store, err := memory.NewOracleMemoryStore(memory.OracleADBConfig{
			HAProxyHost: oracleHost,
			Port:        oraclePort,
			ServiceName: oracleService,
			User:        oracleUser,
			Password:    oraclePass,
		})
		if err != nil {
			log.Printf("[Oracle Memory] ⚠️ Failed to initialize Oracle store: %v", err)
		} else {
			oracleStore = store
		}
	} else {
		log.Printf("[Oracle Memory] ⚪ Oracle memory store not configured in environment, skipping")
	}

	// Matrix Unified Communications & Reaction Client
	var matrixClient *messaging.MatrixClient
	matrixToken := os.Getenv("MATRIX_TOKEN")
	matrixServer := os.Getenv("MATRIX_HOMESERVER")
	if matrixToken != "" {
		if matrixServer == "" {
			matrixServer = "https://matrix.capitaltrain.cn"
		}
		matrixClient = messaging.NewMatrixClient(messaging.MatrixConfig{
			HomeserverURL: matrixServer,
			AccessToken:   matrixToken,
			DefaultRoomID: os.Getenv("MATRIX_ROOM_ID"),
		})
	}

	return &DispatcherService{
		AuthentikURL: authentikURL,
		VaultAddr:    vaultAddr,
		MySQLStore:   mysqlStore,
		OracleStore:  oracleStore,
		MatrixClient: matrixClient,
	}
}

// DispatchTaskResult summarizes the result of processing a Gitea assignment event
type DispatchTaskResult struct {
	AgentName      string            `json:"agent_name"`
	IssueID        int64             `json:"issue_id"`
	TaskTitle      string            `json:"task_title"`
	EstimatedHours float64           `json:"estimated_hours"`
	CalendarStatus *OccupationResult `json:"calendar_status"`
	MemoryLogged   bool              `json:"memory_logged"`
	MatrixEventID  string            `json:"matrix_event_id"`
	MatrixReaction string            `json:"matrix_reaction"`
}

// HandleGiteaWebhook Processing Handler for incoming Gitea webhook requests
func (s *DispatcherService) HandleGiteaWebhook(ctx context.Context, r *http.Request) (*DispatchTaskResult, error) {
	payload, err := ParseGiteaWebhook(r)
	if err != nil {
		return nil, fmt.Errorf("dispatcher: failed to parse webhook: %w", err)
	}

	// 1. Identify Target Agent
	var targetAgent string
	if payload.Assignee != nil && payload.Assignee.Username != "" {
		targetAgent = payload.Assignee.Username
	} else if len(payload.Assignees) > 0 {
		targetAgent = payload.Assignees[0].Username
	} else {
		return nil, fmt.Errorf("dispatcher: no assignee in webhook payload")
	}

	// 2. Extract Duration
	fullText := fmt.Sprintf("%s\n%s", payload.Issue.Title, payload.Issue.Body)
	duration := ExtractDuration(fullText)

	log.Printf("[Dispatcher] Received Task Assignment for Agent [%s] from Gitea (Issue #%d: '%s')",
		targetAgent, payload.Issue.Number, payload.Issue.Title)

	// 3. Log to Memory Bank (Prioritize MySQL for Guest mode, fallback to Oracle ADB)
	memRec := memory.MemoryRecord{
		ID:         fmt.Sprintf("mem-gitea-%d", payload.Issue.Number),
		AgentID:    targetAgent,
		MemoryText: fmt.Sprintf("Gitea Task #%d assigned: '%s'. Duration: %v", payload.Issue.Number, payload.Issue.Title, duration),
		CreatedAt:  time.Now(),
	}

	if s.MySQLStore != nil {
		_ = s.MySQLStore.SaveMemory(ctx, memRec)
		log.Printf("[Memory Bank] Synced Task Metadata to MySQL River1 (Guest Mode)")
	} else if s.OracleStore != nil {
		_ = s.OracleStore.SaveMemory(ctx, memRec)
		log.Printf("[Memory Bank] Synced Task Metadata to Pure Go Oracle ADB 26ai (Host Fallback)")
	}

	// 4. Send Matrix Unified Communications Pager Message & Reaction (m.reaction)
	var matrixEvtID, reactID string
	if s.MatrixClient != nil {
		pagerMsg := fmt.Sprintf("📟 [BP-Pager Notice] Task #%d assigned to Agent [%s]: '%s' (Est: %v)",
			payload.Issue.Number, targetAgent, payload.Issue.Title, duration)
		matrixEvtID, _ = s.MatrixClient.SendMessage(ctx, "", pagerMsg)

		// Attach Reaction emoji 👀 (Acknowledged) & 🚀 (Started)
		reactID, _ = s.MatrixClient.SendReaction(ctx, "", matrixEvtID, "👀")
		_, _ = s.MatrixClient.SendReaction(ctx, "", matrixEvtID, "🚀")

		// Send formal Email notice fallback
		_ = s.MatrixClient.SendEmailNotice(ctx, fmt.Sprintf("%s@capitaltrain.cn", targetAgent),
			fmt.Sprintf("Task Assignment #%d", payload.Issue.Number), pagerMsg)
	}

	// 5. Lock Feishu Calendar Occupation Status
	occReq := OccupationRequest{
		AgentName: targetAgent,
		TaskTitle: payload.Issue.Title,
		IssueURL:  payload.Issue.HTMLURL,
		Duration:  duration,
		StartTime: time.Now(),
	}

	calendarRes, err := CreateOccupationEvent(ctx, occReq)
	if err != nil {
		log.Printf("[Dispatcher] Warning: Calendar occupation failed: %v", err)
	}

	return &DispatchTaskResult{
		AgentName:      targetAgent,
		IssueID:        payload.Issue.Number,
		TaskTitle:      payload.Issue.Title,
		EstimatedHours: duration.Hours(),
		CalendarStatus: calendarRes,
		MemoryLogged:   true,
		MatrixEventID:  matrixEvtID,
		MatrixReaction: reactID,
	}, nil
}
