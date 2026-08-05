// Copyright 2026 Google LLC
//
// Unit tests for Gitea parser & Duration Extractor

package integration_test

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"google.golang.org/adk/v2/integration"
)

func TestExtractDuration(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
	}{
		{"Please process this task, duration: 5h", 5 * time.Hour},
		{"预计工时：5小时", 5 * time.Hour},
		{"Task estimate: 3.5 hours", 3*time.Hour + 30*time.Minute},
		{"No duration specified in issue body", 2 * time.Hour}, // Default 2h
	}

	for _, tt := range tests {
		got := integration.ExtractDuration(tt.input)
		if got != tt.expected {
			t.Errorf("ExtractDuration(%q) = %v, expected %v", tt.input, got, tt.expected)
		}
	}
}

func TestParseGiteaWebhook(t *testing.T) {
	payloadJSON := `{
		"action": "assigned",
		"issue": {
			"id": 101,
			"number": 42,
			"title": "Implement Memory Bank Isolation",
			"body": "Assign to Agent Amber. Estimate: 5h",
			"html_url": "https://gitea.capitaltrain.cn/seekkey/ai-ops/issues/42"
		},
		"assignee": {
			"id": 99,
			"username": "amber",
			"full_name": "Agent Amber"
		}
	}`

	req, err := http.NewRequest("POST", "/webhook/gitea", bytes.NewBufferString(payloadJSON))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	payload, err := integration.ParseGiteaWebhook(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if payload.Action != "assigned" {
		t.Errorf("expected action 'assigned', got %q", payload.Action)
	}
	if payload.Assignee.Username != "amber" {
		t.Errorf("expected assignee 'amber', got %q", payload.Assignee.Username)
	}
}
