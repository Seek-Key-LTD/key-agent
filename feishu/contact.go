// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// ContactUser represents a Feishu user in the organization directory.
type ContactUser struct {
	UserID        string   `json:"user_id"`
	OpenID        string   `json:"open_id"`
	UnionID       string   `json:"union_id"`
	Name          string   `json:"name"`
	JobTitle      string   `json:"job_title"`
	DepartmentIDs []string `json:"department_ids"`
	Email         string   `json:"email"`
	Mobile        string   `json:"mobile"`
}

// GetUser fetches a user from the Feishu directory.
// userID may be a user_id, open_id or union_id depending on userIDType
// ("user_id" | "open_id" | "union_id").
func (c *Client) GetUser(ctx context.Context, userID, userIDType string) (*ContactUser, error) {
	path := fmt.Sprintf("/open-apis/contact/v3/users/%s?user_id_type=%s", userID, userIDType)
	raw, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("feishu: get contact user: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			User ContactUser `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("feishu: parse get contact user response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: get contact user api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return &resp.Data.User, nil
}
