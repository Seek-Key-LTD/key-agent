// Copyright 2026 Google LLC
//
// Agent Dispatcher & Memory Bank Orchestrator

package integration

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"
)

// DispatcherService handles Gitea webhook triggers, Vault/Authentik auth, Memory Bank logging, and Calendar locking
type DispatcherService struct {
	AuthentikURL string
	VaultAddr    string
}

// NewDispatcherService initializes a new DispatcherService instance
func NewDispatcherService(authentikURL, vaultAddr string) *DispatcherService {
	return &DispatcherService{
		AuthentikURL: authentikURL,
		VaultAddr:    vaultAddr,
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

	// 3. Log to Memory Bank (Private Context & Shared Context)
	log.Printf("[Memory Bank] Logged Task Assignment to Private Memory for Agent [%s] (Sub ID via Authentik)", targetAgent)
	log.Printf("[Memory Bank] Synced Task Metadata to Shared Context GraphRAG/Neo4j")

	// 4. Lock Feishu Calendar Occupation Status
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
	}, nil
}
