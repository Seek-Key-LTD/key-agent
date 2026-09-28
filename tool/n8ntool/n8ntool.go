// Copyright 2026 Google LLC
//
// Package n8ntool exposes n8n workflow triggering to the agent as an L1
// functiontool, backed by the integration.N8NClient.

package n8ntool

import (
	"fmt"

	"google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/integration"
	"google.golang.org/adk/v2/tool"
	"google.golang.org/adk/v2/tool/functiontool"
)

// TriggerArgs is the input for the n8n_trigger_workflow tool.
type TriggerArgs struct {
	// WebhookPath is the n8n webhook node path, e.g. "deploy-report".
	WebhookPath string `json:"webhook_path"`
	// Payload is the JSON body forwarded to the workflow.
	Payload map[string]any `json:"payload,omitempty"`
}

// TriggerResult is the output of the n8n_trigger_workflow tool.
type TriggerResult struct {
	// Body is the workflow's HTTP response (usually JSON as a string).
	Body string `json:"body"`
}

// NewTriggerTool returns a tool that triggers an n8n workflow via its webhook
// node and returns the workflow's response body.
func NewTriggerTool(cfg integration.N8NConfig) (tool.Tool, error) {
	client := integration.NewN8NClient(cfg)
	return functiontool.New(functiontool.Config{
		Name:        "n8n_trigger_workflow",
		Description: "Trigger an n8n workflow through its webhook node and return the workflow's response. Use when the task is delegated to an n8n automation pipeline.",
	}, func(ctx agent.Context, args TriggerArgs) (TriggerResult, error) {
		if args.WebhookPath == "" {
			return TriggerResult{}, fmt.Errorf("n8n: webhook_path is required")
		}
		body, err := client.TriggerWebhook(ctx, args.WebhookPath, args.Payload)
		if err != nil {
			return TriggerResult{}, err
		}
		return TriggerResult{Body: string(body)}, nil
	})
}
