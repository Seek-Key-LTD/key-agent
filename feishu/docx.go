// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// createDocxPayload is the request body for creating a docx document.
type createDocxPayload struct {
	Title       string `json:"title"`
	FolderToken string `json:"folder_token,omitempty"`
}

// createDocxResponse is the response from the docx create endpoint.
type createDocxResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		DocumentID string `json:"document_id"`
	} `json:"data"`
}

// CreateDocx creates a new Feishu docx document.
// Returns the document ID on success.
func (c *Client) CreateDocx(ctx context.Context, title, folderToken string) (string, error) {
	payload := createDocxPayload{Title: title, FolderToken: folderToken}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal create docx: %w", err)
	}

	raw, err := c.Do(ctx, "POST", "/open-apis/docx/v1/documents", body)
	if err != nil {
		return "", fmt.Errorf("feishu: create docx: %w", err)
	}

	var resp createDocxResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse create docx response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: create docx api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.DocumentID, nil
}

// FetchDocxRawContent fetches the raw markdown-ish content of a docx document.
func (c *Client) FetchDocxRawContent(ctx context.Context, documentID string) (string, error) {
	path := fmt.Sprintf("/open-apis/docx/v1/documents/%s/raw_content", documentID)
	raw, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return "", fmt.Errorf("feishu: fetch docx raw content: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Content string `json:"content"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse fetch docx response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: fetch docx api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Content, nil
}
