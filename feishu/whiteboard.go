// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// BoardUpdatePayload is the request body for writing code (mermaid/plantuml)
// onto a Feishu whiteboard.
type BoardUpdatePayload struct {
	PlantUMLCode string `json:"plant_uml_code"`
	SyntaxType   int    `json:"syntax_type"`
	ParseMode    int    `json:"parse_mode"`
	Overwrite    bool   `json:"overwrite,omitempty"`
}

// boardUpdateResponse is the response from the board update endpoint.
type boardUpdateResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		CreatedNodeID string `json:"created_node_id"`
	} `json:"data"`
}

// UpdateWhiteboard writes mermaid code to a Feishu whiteboard.
// overwrite=true deletes existing content before writing.
// Returns the created node ID on success.
func (c *Client) UpdateWhiteboard(ctx context.Context, whiteboardToken, mermaidCode string, overwrite bool) (string, error) {
	payload := BoardUpdatePayload{
		PlantUMLCode: mermaidCode,
		SyntaxType:   2, // 2 = mermaid
		ParseMode:    1,
		Overwrite:    overwrite,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal update whiteboard: %w", err)
	}

	path := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes/plantuml", whiteboardToken)
	raw, err := c.Do(ctx, "POST", path, body)
	if err != nil {
		return "", fmt.Errorf("feishu: update whiteboard: %w", err)
	}

	var resp boardUpdateResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse update whiteboard response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: update whiteboard api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.CreatedNodeID, nil
}

// ExportWhiteboardImage downloads the preview image of a whiteboard
// as raw bytes (PNG).
func (c *Client) ExportWhiteboardImage(ctx context.Context, whiteboardToken string) ([]byte, error) {
	path := fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", whiteboardToken)
	raw, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("feishu: export whiteboard image: %w", err)
	}
	return raw, nil
}
