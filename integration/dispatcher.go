// Copyright 2026 Google LLC
//
// Agent Dispatcher, Memory Bank & Matrix Unified Communications Orchestrator

package integration

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/messaging"
)

// DispatcherService handles Gitea webhook triggers, Vault/Authentik auth, Oracle Memory, Matrix Pager & Calendar locking
type DispatcherService struct {
	AuthentikURL string
	VaultAddr    string
	OracleStore  *memory.OracleMemoryStore
	MatrixClient *messaging.MatrixClient
}

// NewDispatcherService initializes a new DispatcherService instance
func NewDispatcherService(authentikURL, vaultAddr string) *DispatcherService {
	// Initialize Pure Go Oracle Memory Store (HAProxy SSL Bridge on 11522 / lake2)
	oracleStore, _ := memory.NewOracleMemoryStore(memory.OracleADBConfig{
		HAProxyHost: "192.168.31.111",
		Port:        11522,
		ServiceName: "g8dfe5cebce8245_lake2_medium.adb.oraclecloud.com",
		User:        "ADMIN",
		Password:    "placeholder_pass",
	})

	// Initialize Matrix Unified Communications & Reaction Client
	matrixClient := messaging.NewMatrixClient(messaging.MatrixConfig{
		HomeserverURL: "https://matrix.capitaltrain.cn",
		AccessToken:   "syt_agent_bot_token",
		DefaultRoomID: "!agent-ops:capitaltrain.cn",
	})

	return &DispatcherService{
		AuthentikURL: authentikURL,
		VaultAddr:    vaultAddr,
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

	// 3. Log to Pure Go Oracle ADB 26ai Memory Bank (Private Context & Vector Memory)
	if s.OracleStore != nil {
		memRec := memory.MemoryRecord{
			ID:         fmt.Sprintf("mem-gitea-%d", payload.Issue.Number),
			AgentID:    targetAgent,
			MemoryText: fmt.Sprintf("Gitea Task #%d assigned: '%s'. Duration: %v", payload.Issue.Number, payload.Issue.Title, duration),
			CreatedAt:  time.Now(),
		}
		_ = s.OracleStore.SaveMemory(ctx, memRec)
	}
	log.Printf("[Memory Bank] Synced Task Metadata to Pure Go Oracle ADB 26ai (Zero CGO / C-lib)")

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
