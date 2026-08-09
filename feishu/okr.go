// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// OkrPeriod represents an OKR cycle/period.
type OkrPeriod struct {
	PeriodID string `json:"period_id"`
	Name     string `json:"name"`
	Start    string `json:"start"`
	End      string `json:"end"`
}

// ListOkrPeriods lists OKR cycles of a user.
func (c *Client) ListOkrPeriods(ctx context.Context, userID string) ([]OkrPeriod, error) {
	path := "/open-apis/okr/v1/periods"
	if userID != "" {
		path += "?user_id=" + userID
	}
	raw, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("feishu: list okr periods: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []OkrPeriod `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("feishu: parse list okr periods response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: list okr periods api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Items, nil
}

// CreateOkrAlignment creates an alignment from one OKR entity to another.
// objectiveID is the current objective; toEntityID/toEntityType is the
// entity being aligned to (type: 1 = objective, 2 = key result).
func (c *Client) CreateOkrAlignment(ctx context.Context, objectiveID, toEntityID string, toEntityType int) error {
	payload := map[string]interface{}{
		"to_entity_id":   toEntityID,
		"to_entity_type": toEntityType,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("feishu: marshal create okr alignment: %w", err)
	}

	path := fmt.Sprintf("/open-apis/okr/v1/objectives/%s/alignments", objectiveID)
	raw, err := c.Do(ctx, "POST", path, body)
	if err != nil {
		return fmt.Errorf("feishu: create okr alignment: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("feishu: parse create okr alignment response: %w", err)
	}
	if resp.Code != 0 {
		return fmt.Errorf("feishu: create okr alignment api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return nil
}
