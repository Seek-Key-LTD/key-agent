// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// CreatePresentation creates a Feishu slides presentation from slide XML strings.
// slides is a JSON array, each element a <slide> XML string (max 10).
// Returns the presentation ID on success.
func (c *Client) CreatePresentation(ctx context.Context, title string, slides []string) (string, error) {
	payload := map[string]interface{}{
		"title":  title,
		"slides": slides,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal create presentation: %w", err)
	}

	raw, err := c.Do(ctx, "POST", "/open-apis/slides_ai/v1/xml_presentations", body)
	if err != nil {
		return "", fmt.Errorf("feishu: create presentation: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			PresentationID string `json:"presentation_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse create presentation response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: create presentation api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.PresentationID, nil
}
