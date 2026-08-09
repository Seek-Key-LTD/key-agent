// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// CreateTask creates a task in Feishu Task v2.
// Returns the task GUID on success.
func (c *Client) CreateTask(ctx context.Context, summary, description string) (string, error) {
	payload := map[string]interface{}{
		"summary":     summary,
		"description": description,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal create task: %w", err)
	}

	raw, err := c.Do(ctx, "POST", "/open-apis/task/v2/tasks", body)
	if err != nil {
		return "", fmt.Errorf("feishu: create task: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Task struct {
				GUID string `json:"guid"`
			} `json:"task"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse create task response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: create task api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Task.GUID, nil
}
