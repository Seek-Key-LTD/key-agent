// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// ApprovalInstanceStatus is the status of an approval instance.
type ApprovalInstanceStatus struct {
	InstanceID string `json:"instance_id"`
	Status     string `json:"status"`
	StatusText string `json:"status_text"`
	Title      string `json:"title"`
}

// GetApprovalInstance fetches an approval instance by ID.
func (c *Client) GetApprovalInstance(ctx context.Context, instanceID, userID string) (*ApprovalInstanceStatus, error) {
	path := fmt.Sprintf("/open-apis/approval/v4/instances/%s", instanceID)
	if userID != "" {
		path += "?user_id=" + userID
	}
	raw, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("feishu: get approval instance: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Instance ApprovalInstanceStatus `json:"instance"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("feishu: parse get approval instance response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: get approval instance api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return &resp.Data.Instance, nil
}
