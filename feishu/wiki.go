// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// WikiSpace represents a Feishu wiki knowledge space.
type WikiSpace struct {
	SpaceID string `json:"space_id"`
	Name    string `json:"name"`
}

// ListSpaces lists wiki knowledge spaces visible to the app.
func (c *Client) ListSpaces(ctx context.Context) ([]WikiSpace, error) {
	raw, err := c.Do(ctx, "GET", "/open-apis/wiki/v2/spaces", nil)
	if err != nil {
		return nil, fmt.Errorf("feishu: list wiki spaces: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []WikiSpace `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("feishu: parse list wiki spaces response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: list wiki spaces api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Items, nil
}
