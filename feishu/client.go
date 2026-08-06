// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.
package feishu

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	baseURL = "https://open.feishu.cn"
)

// Config holds the Feishu app credentials.
type Config struct {
	AppID     string
	AppSecret string
}

// Client is a Feishu Open API client with automatic token caching.
type Client struct {
	cfg       Config
	mu        sync.RWMutex
	token     string
	expiresAt time.Time
	http      *http.Client
}

// New creates a new Feishu client.
func New(cfg Config) *Client {
	return &Client{
		cfg:  cfg,
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// tenantAccessTokenResponse is the response from the token endpoint.
type tenantAccessTokenResponse struct {
	Code              int    `json:"code"`
	Msg               string `json:"msg"`
	TenantAccessToken string `json:"tenant_access_token"`
	Expire            int    `json:"expire"`
}

// TenantAccessToken returns a valid tenant access token, refreshing it if needed.
func (c *Client) TenantAccessToken(ctx context.Context) (string, error) {
	c.mu.RLock()
	if c.token != "" && time.Now().Before(c.expiresAt) {
		t := c.token
		c.mu.RUnlock()
		return t, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock
	if c.token != "" && time.Now().Before(c.expiresAt) {
		return c.token, nil
	}

	body, err := json.Marshal(map[string]string{
		"app_id":     c.cfg.AppID,
		"app_secret": c.cfg.AppSecret,
	})
	if err != nil {
		return "", fmt.Errorf("feishu: marshal token request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/open-apis/auth/v3/tenant_access_token/internal",
		bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("feishu: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("feishu: token request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("feishu: read token response: %w", err)
	}

	var tr tenantAccessTokenResponse
	if err := json.Unmarshal(raw, &tr); err != nil {
		return "", fmt.Errorf("feishu: parse token response: %w", err)
	}
	if tr.Code != 0 {
		return "", fmt.Errorf("feishu: token api error code=%d msg=%s", tr.Code, tr.Msg)
	}

	c.token = tr.TenantAccessToken
	c.expiresAt = time.Now().Add(time.Duration(tr.Expire) * time.Second)
	return c.token, nil
}

// Do performs an authenticated request to the Feishu Open API.
// The path should be the API path starting with "/open-apis/...".
func (c *Client) Do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	token, err := c.TenantAccessToken(ctx)
	if err != nil {
		return nil, err
	}

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return nil, fmt.Errorf("feishu: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("feishu: request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("feishu: read response: %w", err)
	}

	// Check Feishu API-level error
	var apiErr struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(raw, &apiErr); err == nil && apiErr.Code != 0 {
		return nil, fmt.Errorf("feishu: api error code=%d msg=%s", apiErr.Code, apiErr.Msg)
	}

	return raw, nil
}