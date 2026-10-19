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

package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
)

// ErrClosed indicates that the client was explicitly closed.
var ErrClosed = errors.New("acp: client closed")

type outgoing struct {
	ctx     context.Context
	message any
}

// Client multiplexes concurrent JSON-RPC requests over a newline-delimited
// transport. The transport's Close must unblock Read and Write. Call Close to
// release resources. Context cancellation stops waiting locally; it does not
// cancel a prompt already being processed by the agent.
type Client struct {
	transport io.ReadWriteCloser
	mu        sync.Mutex
	nextID    uint64
	pending   map[string]chan Response
	err       error
	closeErr  error
	once      sync.Once
	done      chan struct{}
	out       chan outgoing
	notices   chan Notification
	wg        sync.WaitGroup
}

// NewClient takes ownership of transport and starts its read and write loops.
func NewClient(transport io.ReadWriteCloser) *Client {
	c := &Client{
		transport: transport,
		pending:   make(map[string]chan Response),
		done:      make(chan struct{}),
		out:       make(chan outgoing),
		notices:   make(chan Notification, 64),
	}
	c.wg.Add(2)
	go c.readLoop()
	go c.writeLoop()
	return c
}

// NewStdioClient connects to an agent's stdout (reader) and stdin (writer).
// It owns both pipes; process creation and lifecycle remain with the caller.
func NewStdioClient(reader io.ReadCloser, writer io.WriteCloser) *Client {
	return NewClient(&stdioTransport{ReadCloser: reader, writer: writer})
}

type stdioTransport struct {
	io.ReadCloser
	writer io.WriteCloser
}

func (s *stdioTransport) Write(p []byte) (int, error) { return s.writer.Write(p) }

func (s *stdioTransport) Close() error {
	return errors.Join(s.ReadCloser.Close(), s.writer.Close())
}

// Notifications yields agent notifications, including streamed session updates.
// Drain this channel while using the client. If its 64-message buffer fills,
// the connection fails instead of silently losing output. Close ends the stream.
func (c *Client) Notifications() <-chan Notification { return c.notices }

// Initialize negotiates ACP v1. Unsupported versions close the connection.
func (c *Client) Initialize(ctx context.Context, req InitializeRequest) (*InitializeResponse, error) {
	if req.ProtocolVersion == 0 {
		req.ProtocolVersion = ProtocolVersion
	}
	if req.ClientCapabilities == nil {
		req.ClientCapabilities = map[string]any{}
	}
	var result InitializeResponse
	if err := c.call(ctx, "initialize", req, &result); err != nil {
		return nil, err
	}
	if result.ProtocolVersion != ProtocolVersion {
		err := fmt.Errorf("acp: unsupported protocol version %d", result.ProtocolVersion)
		c.shutdown(err)
		return nil, err
	}
	return &result, nil
}

// NewSession creates a session after initialization has completed.
func (c *Client) NewSession(ctx context.Context, req NewSessionRequest) (*NewSessionResponse, error) {
	if req.MCPServers == nil {
		req.MCPServers = []map[string]any{}
	}
	var result NewSessionResponse
	if err := c.call(ctx, "session/new", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SendPrompt waits for a turn to finish. Consume Notifications concurrently to
// receive the agent's streamed output. The caller must first create a session.
func (c *Client) SendPrompt(ctx context.Context, req PromptRequest) (*PromptResponse, error) {
	var result PromptResponse
	if err := c.call(ctx, "session/prompt", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Close is idempotent and waits for both transport loops to exit.
func (c *Client) Close() error {
	c.shutdown(ErrClosed)
	c.wg.Wait()
	return c.closeErr
}

func (c *Client) shutdown(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		c.pending = make(map[string]chan Response)
		close(c.done)
		c.mu.Unlock()
		c.closeErr = c.transport.Close()
	})
}

func (c *Client) failure() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}

func (c *Client) call(ctx context.Context, method string, params, result any) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("acp: encode %s: %w", method, err)
	}
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.nextID++
	id := strconv.FormatUint(c.nextID, 10)
	response := make(chan Response, 1)
	c.pending[id] = response
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	request := Request{JSONRPC: "2.0", ID: json.RawMessage(id), Method: method, Params: data}
	select {
	case c.out <- outgoing{ctx: ctx, message: request}:
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		return c.failure()
	}
	select {
	case resp := <-response:
		return decodeResult(method, resp, result)
	case <-ctx.Done():
		return ctx.Err()
	case <-c.done:
		// A peer may close immediately after its final reply. Prefer a reply
		// already delivered by the reader over the subsequent EOF.
		select {
		case resp := <-response:
			return decodeResult(method, resp, result)
		default:
		}
		return c.failure()
	}
}

func decodeResult(method string, resp Response, result any) error {
	if resp.Error != nil {
		return resp.Error
	}
	if err := json.Unmarshal(resp.Result, result); err != nil {
		return fmt.Errorf("acp: decode %s result: %w", method, err)
	}
	return nil
}

func (c *Client) writeLoop() {
	defer c.wg.Done()
	encoder := json.NewEncoder(c.transport)
	for {
		select {
		case <-c.done:
			return
		case msg := <-c.out:
			if msg.ctx.Err() != nil {
				continue
			}
			if err := encoder.Encode(msg.message); err != nil {
				c.shutdown(fmt.Errorf("acp: write: %w", err))
				return
			}
		}
	}
}

func (c *Client) readLoop() {
	defer c.wg.Done()
	defer close(c.notices)
	reader := bufio.NewReader(c.transport)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			c.shutdown(fmt.Errorf("acp: read: %w", err))
			return
		}
		var msg struct {
			Response
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			c.shutdown(fmt.Errorf("acp: decode message: %w", err))
			return
		}
		if msg.JSONRPC != "2.0" {
			c.shutdown(errors.New("acp: invalid JSON-RPC version"))
			return
		}
		if msg.Method != "" {
			if len(msg.ID) != 0 {
				// No filesystem, terminal or permission capabilities are implemented.
				resp := Response{JSONRPC: "2.0", ID: msg.ID, Error: &Error{Code: -32601, Message: "Method not found"}}
				select {
				case c.out <- outgoing{ctx: context.Background(), message: resp}:
				case <-c.done:
					return
				}
				continue
			}
			select {
			case c.notices <- Notification{JSONRPC: msg.JSONRPC, Method: msg.Method, Params: msg.Params}:
			default:
				c.shutdown(errors.New("acp: notification buffer full"))
				return
			}
			continue
		}
		if len(msg.ID) == 0 || (msg.Result == nil) == (msg.Error == nil) {
			c.shutdown(errors.New("acp: invalid JSON-RPC response"))
			return
		}
		c.mu.Lock()
		pending := c.pending[string(msg.ID)]
		delete(c.pending, string(msg.ID))
		c.mu.Unlock()
		if pending != nil {
			pending <- msg.Response
		}
	}
}
