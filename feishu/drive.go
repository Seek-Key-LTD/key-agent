// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// DriveFile represents a file or folder in Feishu Drive.
type DriveFile struct {
	FileToken string `json:"file_token"`
	Name      string `json:"name"`
	Type      string `json:"type"`
}

// ListFiles lists files and folders under a drive folder.
// An empty folderToken lists root-level files.
func (c *Client) ListFiles(ctx context.Context, folderToken string) ([]DriveFile, error) {
	path := "/open-apis/drive/v1/files"
	if folderToken != "" {
		path += "?folder_token=" + folderToken
	}
	raw, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("feishu: list drive files: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Files []DriveFile `json:"files"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("feishu: parse list drive files response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: list drive files api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Files, nil
}
