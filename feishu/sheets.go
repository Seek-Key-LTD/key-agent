// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// CreateSpreadsheet creates a new Feishu spreadsheet.
// Returns the spreadsheet token on success.
func (c *Client) CreateSpreadsheet(ctx context.Context, title string) (string, error) {
	body, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return "", fmt.Errorf("feishu: marshal create spreadsheet: %w", err)
	}

	raw, err := c.Do(ctx, "POST", "/open-apis/sheets/v3/spreadsheets", body)
	if err != nil {
		return "", fmt.Errorf("feishu: create spreadsheet: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Spreadsheet struct {
				SpreadsheetToken string `json:"spreadsheet_token"`
			} `json:"spreadsheet"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse create spreadsheet response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: create spreadsheet api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Spreadsheet.SpreadsheetToken, nil
}

// ReadSheetValues reads a range of cell values from a spreadsheet.
// sheetRange uses the A1 notation, e.g. "Sheet1!A1:C10".
func (c *Client) ReadSheetValues(ctx context.Context, spreadsheetToken, sheetRange string) ([][]interface{}, error) {
	path := fmt.Sprintf("/open-apis/sheets/v3/spreadsheets/%s/values/%s", spreadsheetToken, sheetRange)
	raw, err := c.Do(ctx, "GET", path, nil)
	if err != nil {
		return nil, fmt.Errorf("feishu: read sheet values: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			ValueRange struct {
				Values [][]interface{} `json:"values"`
			} `json:"value_range"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("feishu: parse read sheet values response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: read sheet values api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.ValueRange.Values, nil
}
