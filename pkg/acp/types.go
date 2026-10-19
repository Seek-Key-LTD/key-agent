// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package acp implements a lightweight ACP v1 (Agent Client Protocol) client.
package acp

import (
	"encoding/json"
	"fmt"
)

// ProtocolVersion is the supported ACP major version.
const ProtocolVersion = 1

// Request is a JSON-RPC 2.0 request. IDs can be numbers or strings.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response with either Result or Error set.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a remote JSON-RPC error, including optional structured data.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *Error) Error() string {
	return fmt.Sprintf("acp: RPC error %d: %s", e.Code, e.Message)
}

// Notification is a JSON-RPC message without an ID or response.
type Notification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Implementation identifies a client or agent.
type Implementation struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Title   string `json:"title,omitempty"`
}

// InitializeRequest negotiates the version and optional client capabilities.
// A zero ProtocolVersion defaults to ProtocolVersion; nil capabilities become {}.
type InitializeRequest struct {
	ProtocolVersion    int             `json:"protocolVersion"`
	ClientCapabilities map[string]any  `json:"clientCapabilities"`
	ClientInfo         *Implementation `json:"clientInfo,omitempty"`
}

// InitializeResponse describes the agent's version, capabilities and auth methods.
type InitializeResponse struct {
	ProtocolVersion   int              `json:"protocolVersion"`
	AgentCapabilities map[string]any   `json:"agentCapabilities"`
	AgentInfo         *Implementation  `json:"agentInfo,omitempty"`
	AuthMethods       []Authentication `json:"authMethods"`
}

// Authentication describes an authentication method offered by the agent.
type Authentication struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// NewSessionRequest creates a session in an absolute working directory.
// MCPServers contains transport-specific ACP MCP server objects; nil becomes [].
type NewSessionRequest struct {
	CWD        string           `json:"cwd"`
	MCPServers []map[string]any `json:"mcpServers"`
}

// NewSessionResponse identifies the newly created session.
type NewSessionResponse struct {
	SessionID     string          `json:"sessionId"`
	Modes         json.RawMessage `json:"modes,omitempty"`
	Models        json.RawMessage `json:"models,omitempty"`
	ConfigOptions json.RawMessage `json:"configOptions,omitempty"`
}

// PromptRequest starts a prompt turn for a session.
type PromptRequest struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

// PromptResponse completes a turn. Streamed output arrives as session/update
// notifications, available from Client.Notifications.
type PromptResponse struct {
	StopReason string          `json:"stopReason"`
	Usage      json.RawMessage `json:"usage,omitempty"`
}

// ContentBlock represents text, image, audio, resource or resource_link content.
// Set the fields appropriate to Type; Resource holds an embedded resource object.
type ContentBlock struct {
	Type        string          `json:"type"`
	Text        string          `json:"text,omitempty"`
	Data        string          `json:"data,omitempty"`
	MimeType    string          `json:"mimeType,omitempty"`
	URI         string          `json:"uri,omitempty"`
	Name        string          `json:"name,omitempty"`
	Title       string          `json:"title,omitempty"`
	Description string          `json:"description,omitempty"`
	Size        *int64          `json:"size,omitempty"`
	Resource    json.RawMessage `json:"resource,omitempty"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}

// MarshalJSON preserves the required text field even for an empty text block.
func (b ContentBlock) MarshalJSON() ([]byte, error) {
	type block ContentBlock
	if b.Type == "text" {
		return json.Marshal(struct {
			block
			Text string `json:"text"`
		}{block: block(b), Text: b.Text})
	}
	return json.Marshal(block(b))
}
