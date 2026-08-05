// Copyright 2026 Google LLC
//
// Gitea Webhook Event Parser & Task Assignment Extractor

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// GiteaIssuePayload represents the webhook structure sent by Gitea on Issue assignment
type GiteaIssuePayload struct {
	Action     string        `json:"action"` // "assigned", "opened", etc.
	Issue      GiteaIssue    `json:"issue"`
	Repository GiteaRepo     `json:"repository"`
	Sender     GiteaUser     `json:"sender"`
	Assignee   *GiteaUser    `json:"assignee,omitempty"`
	Assignees  []GiteaUser   `json:"assignees,omitempty"`
}

type GiteaIssue struct {
	ID          int64       `json:"id"`
	Number      int64       `json:"number"`
	Title       string      `json:"title"`
	Body        string      `json:"body"`
	State       string      `json:"state"`
	HTMLURL     string      `json:"html_url"`
	Assignee    *GiteaUser  `json:"assignee,omitempty"`
	Assignees   []GiteaUser `json:"assignees,omitempty"`
}

type GiteaRepo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	HTMLURL  string `json:"html_url"`
}

type GiteaUser struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	FullName string `json:"full_name"`
	Email    string `json:"email"`
}

// ParseGiteaWebhook parses an incoming Gitea Webhook HTTP request
func ParseGiteaWebhook(r *http.Request) (*GiteaIssuePayload, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("empty request body")
	}
	defer r.Body.Close()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}

	var payload GiteaIssuePayload
	if err := json.Unmarshal(bodyBytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Gitea webhook JSON: %w", err)
	}

	return &payload, nil
}

// ExtractDuration parses estimated task duration from issue title or body text.
// Examples supported: "5h", "5小时", "3.5 hours", "工时: 4h"
// Defaults to 2 hours if not explicitly mentioned.
func ExtractDuration(text string) time.Duration {
	// Regular expressions for duration matching
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:h|hr|hrs|hours|小时)`),
		regexp.MustCompile(`(?i)(?:工时|时长|duration|estimate)[:：]\s*(\d+(?:\.\d+)?)`),
	}

	for _, re := range patterns {
		matches := re.FindStringSubmatch(text)
		if len(matches) > 1 {
			if hours, err := strconv.ParseFloat(matches[1], 64); err == nil && hours > 0 {
				return time.Duration(hours * float64(time.Hour))
			}
		}
	}

	// Default fallback: 2 hours
	return 2 * time.Hour
}
