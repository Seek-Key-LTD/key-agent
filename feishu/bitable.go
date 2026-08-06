// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// addRecordPayload is the request body for adding a bitable record.
type addRecordPayload struct {
	Fields map[string]interface{} `json:"fields"`
}

// recordResponse is the common response envelope for bitable record operations.
type recordResponse struct {
	Code int                    `json:"code"`
	Msg  string                 `json:"msg"`
	Data map[string]interface{} `json:"data"`
}

// AddRecord adds a record to a Feishu bitable table.
// Returns the record ID on success.
func (c *Client) AddRecord(ctx context.Context, appToken, tableID string, fields map[string]interface{}) (string, error) {
	payload := addRecordPayload{Fields: fields}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal add record: %w", err)
	}

	path := fmt.Sprintf("/open-apis/bitable/v1/apps/%s/tables/%s/records", appToken, tableID)
	raw, err := c.Do(ctx, "POST", path, body)
	if err != nil {
		return "", fmt.Errorf("feishu: add record: %w", err)
	}

	var resp recordResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse add record response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: add record api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	recordID, _ := resp.Data["record_id"].(string)
	if recordID == "" {
		return "", fmt.Errorf("feishu: add record response missing record_id")
	}

	return recordID, nil
}

// UpdateRecord updates an existing record in a Feishu bitable table.
func (c *Client) UpdateRecord(ctx context.Context, appToken, tableID, recordID string, fields map[string]interface{}) error {
	payload := addRecordPayload{Fields: fields}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("feishu: marshal update record: %w", err)
	}

	path := fmt.Sprintf("/open-apis/bitable/v1/apps/%s/tables/%s/records/%s", appToken, tableID, recordID)
	raw, err := c.Do(ctx, "PUT", path, body)
	if err != nil {
		return fmt.Errorf("feishu: update record: %w", err)
	}

	var resp recordResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return fmt.Errorf("feishu: parse update record response: %w", err)
	}
	if resp.Code != 0 {
		return fmt.Errorf("feishu: update record api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return nil
}