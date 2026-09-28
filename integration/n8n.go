// Copyright 2026 Google LLC
//
// n8n integration: REST API client (webhook trigger, execution query, workflow
// list) and inbound webhook payload parser. No external dependencies.

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// N8NConfig holds n8n connection settings.
type N8NConfig struct {
	// BaseURL of the n8n instance, e.g. "https://n8n.capitaltrain.cn".
	BaseURL string
	// APIKey enables the /api/v1 management API (executions, workflows).
	// Webhook triggering does not need it when the webhook node is public.
	APIKey string
	// HTTPTimeout defaults to 30s when zero.
	HTTPTimeout time.Duration
}

// N8NClient is an n8n REST client.
type N8NClient struct {
	cfg  N8NConfig
	http *http.Client
}

// NewN8NClient creates an n8n client.
func NewN8NClient(cfg N8NConfig) *N8NClient {
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 30 * time.Second
	}
	return &N8NClient{cfg: cfg, http: &http.Client{Timeout: cfg.HTTPTimeout}}
}

// TriggerWebhook fires an n8n workflow through its webhook node path, e.g.
// webhookPath "deploy-report" hits POST {BaseURL}/webhook/deploy-report.
// n8n webhook nodes are either public (no auth) or Basic-Auth protected; set
// the auth on the workflow side. Returns the workflow's HTTP response body.
func (c *N8NClient) TriggerWebhook(ctx context.Context, webhookPath string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("n8n: marshal payload: %w", err)
	}
	url := fmt.Sprintf("%s/webhook/%s", c.cfg.BaseURL, webhookPath)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("n8n: create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.do(req)
}

// N8NExecution is the subset of an n8n execution object used here.
type N8NExecution struct {
	ID         string          `json:"id"`
	WorkflowID string          `json:"workflowId"`
	Status     string          `json:"status"` // "error" | "success" | "waiting" | "running" ...
	Mode       string          `json:"mode"`
	StartedAt  time.Time       `json:"startedAt"`
	StoppedAt  *time.Time      `json:"stoppedAt"`
	Raw        json.RawMessage `json:"-"`
}

// GetExecution fetches an execution by ID (requires APIKey).
func (c *N8NClient) GetExecution(ctx context.Context, id string) (*N8NExecution, error) {
	raw, err := c.apiGet(ctx, "/api/v1/executions/"+id)
	if err != nil {
		return nil, err
	}
	var ex N8NExecution
	if err := json.Unmarshal(raw, &ex); err != nil {
		return nil, fmt.Errorf("n8n: parse execution: %w", err)
	}
	ex.Raw = raw
	return &ex, nil
}

// N8NWorkflow is the subset of an n8n workflow object used here.
type N8NWorkflow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	UpdatedAt string `json:"updatedAt"`
}

// ListWorkflows lists workflows (requires APIKey).
func (c *N8NClient) ListWorkflows(ctx context.Context) ([]N8NWorkflow, error) {
	raw, err := c.apiGet(ctx, "/api/v1/workflows?limit=100")
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []N8NWorkflow `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("n8n: parse workflows: %w", err)
	}
	return resp.Data, nil
}

// N8NWebhookPayload is the shape n8n sends when a workflow calls back into the
// agent (HTTP Request node pointing at us).
type N8NWebhookPayload struct {
	ExecutionID string          `json:"executionId"`
	WorkflowID  string          `json:"workflowId"`
	Event       string          `json:"event"`
	Data        json.RawMessage `json:"data"`
}

// ParseN8NWebhook parses an inbound n8n callback HTTP request.
func ParseN8NWebhook(r *http.Request) (*N8NWebhookPayload, error) {
	if r.Body == nil {
		return nil, fmt.Errorf("n8n: empty request body")
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("n8n: read body: %w", err)
	}
	var p N8NWebhookPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("n8n: parse webhook JSON: %w", err)
	}
	return &p, nil
}

// apiGet performs an authenticated GET against the /api/v1 management API.
func (c *N8NClient) apiGet(ctx context.Context, path string) ([]byte, error) {
	if c.cfg.APIKey == "" {
		return nil, fmt.Errorf("n8n: APIKey required for %s", path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.BaseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("n8n: create request: %w", err)
	}
	req.Header.Set("X-N8N-API-KEY", c.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
	return c.do(req)
}

// do executes the request and returns the body, mapping HTTP errors.
func (c *N8NClient) do(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("n8n: request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("n8n: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("n8n: http %d: %s", resp.StatusCode, truncateForError(raw))
	}
	return raw, nil
}

func truncateForError(b []byte) string {
	const max = 512
	if len(b) > max {
		return string(b[:max]) + "...(truncated)"
	}
	return string(b)
}
