// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// SendMessage sends a message via Feishu IM.
// receiveIDType is one of: open_id | user_id | chat_id | email | union_id.
// msgType is one of: text | post | image | interactive | ...
// content is the msg_type-specific JSON string (e.g. {"text":"hello"}).
// Returns the message ID on success.
func (c *Client) SendMessage(ctx context.Context, receiveID, receiveIDType, msgType, content string) (string, error) {
	payload := map[string]string{
		"receive_id": receiveID,
		"msg_type":   msgType,
		"content":    content,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal send message: %w", err)
	}

	path := fmt.Sprintf("/open-apis/im/v1/messages?receive_id_type=%s", receiveIDType)
	raw, err := c.Do(ctx, "POST", path, body)
	if err != nil {
		return "", fmt.Errorf("feishu: send message: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse send message response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: send message api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.MessageID, nil
}
