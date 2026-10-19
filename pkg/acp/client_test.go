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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

func pipeClient(t *testing.T) (*Client, net.Conn, *json.Decoder, *json.Encoder) {
	t.Helper()
	clientConn, server := net.Pipe()
	client := NewClient(clientConn)
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(func() { _ = server.Close() })
	if err := server.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	return client, server, json.NewDecoder(server), json.NewEncoder(server)
}

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func readRequest(t *testing.T, decoder *json.Decoder, method string) Request {
	t.Helper()
	var req Request
	if err := decoder.Decode(&req); err != nil {
		t.Fatal(err)
	}
	if req.JSONRPC != "2.0" || len(req.ID) == 0 || req.Method != method {
		t.Fatalf("unexpected request: %+v", req)
	}
	return req
}

func reply(t *testing.T, encoder *json.Encoder, req Request, result string) {
	t.Helper()
	if err := encoder.Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(result)}); err != nil {
		t.Fatal(err)
	}
}

func TestInitializeNewSessionSendPrompt(t *testing.T) {
	client, _, dec, enc := pipeClient(t)
	ctx := testContext(t)
	finished := make(chan error, 1)
	go func() {
		init, err := client.Initialize(ctx, InitializeRequest{ClientInfo: &Implementation{Name: "key-agent", Version: "1"}})
		if err != nil {
			finished <- err
			return
		}
		if init.ProtocolVersion != 1 || init.AgentInfo.Name != "test-agent" || len(init.AuthMethods) != 1 {
			finished <- fmt.Errorf("unexpected initialize result: %+v", init)
			return
		}
		session, err := client.NewSession(ctx, NewSessionRequest{CWD: "/workspace"})
		if err != nil {
			finished <- err
			return
		}
		if session.SessionID != "session-1" {
			finished <- fmt.Errorf("unexpected session: %+v", session)
			return
		}
		prompt, err := client.SendPrompt(ctx, PromptRequest{SessionID: session.SessionID, Prompt: []ContentBlock{{Type: "text", Text: "hello"}}})
		if err == nil && prompt.StopReason != "end_turn" {
			err = fmt.Errorf("unexpected prompt response: %+v", prompt)
		}
		finished <- err
	}()

	req := readRequest(t, dec, "initialize")
	var init map[string]any
	if err := json.Unmarshal(req.Params, &init); err != nil {
		t.Fatal(err)
	}
	if init["protocolVersion"] != float64(1) || init["clientCapabilities"] == nil {
		t.Fatalf("invalid initialization defaults: %s", req.Params)
	}
	reply(t, enc, req, `{"protocolVersion":1,"agentCapabilities":{},"agentInfo":{"name":"test-agent","version":"1"},"authMethods":[{"id":"token","name":"Token"}]}`)

	req = readRequest(t, dec, "session/new")
	var session map[string]any
	if err := json.Unmarshal(req.Params, &session); err != nil {
		t.Fatal(err)
	}
	servers, ok := session["mcpServers"].([]any)
	if session["cwd"] != "/workspace" || !ok || len(servers) != 0 {
		t.Fatalf("unexpected session params: %s", req.Params)
	}
	reply(t, enc, req, `{"sessionId":"session-1"}`)

	req = readRequest(t, dec, "session/prompt")
	var prompt PromptRequest
	if err := json.Unmarshal(req.Params, &prompt); err != nil {
		t.Fatal(err)
	}
	if prompt.SessionID != "session-1" || len(prompt.Prompt) != 1 || prompt.Prompt[0].Text != "hello" {
		t.Fatalf("unexpected prompt params: %s", req.Params)
	}
	if err := enc.Encode(Notification{JSONRPC: "2.0", Method: "session/update", Params: json.RawMessage(`{"sessionId":"session-1","update":{"sessionUpdate":"agent_message_chunk","content":{"type":"text","text":"hi"}}}`)}); err != nil {
		t.Fatal(err)
	}
	reply(t, enc, req, `{"stopReason":"end_turn"}`)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
	select {
	case notice := <-client.Notifications():
		if notice.Method != "session/update" || !strings.Contains(string(notice.Params), `"hi"`) {
			t.Fatalf("unexpected notification: %+v", notice)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestConcurrentRequestsMatchOutOfOrder(t *testing.T) {
	client, _, dec, enc := pipeClient(t)
	ctx := testContext(t)
	const count = 20
	results := make(chan error, count)
	for i := range count {
		go func() {
			cwd := fmt.Sprintf("/workspace/%d", i)
			resp, err := client.NewSession(ctx, NewSessionRequest{CWD: cwd})
			if err == nil && resp.SessionID != cwd {
				err = fmt.Errorf("got session %q, want %q", resp.SessionID, cwd)
			}
			results <- err
		}()
	}
	requests := make([]Request, count)
	ids := map[string]bool{}
	for i := range count {
		requests[i] = readRequest(t, dec, "session/new")
		id := string(requests[i].ID)
		if ids[id] {
			t.Fatalf("duplicate request ID %s", id)
		}
		ids[id] = true
	}
	for i := count - 1; i >= 0; i-- {
		var params NewSessionRequest
		if err := json.Unmarshal(requests[i].Params, &params); err != nil {
			t.Fatal(err)
		}
		result, err := json.Marshal(NewSessionResponse{SessionID: params.CWD})
		if err != nil {
			t.Fatal(err)
		}
		reply(t, enc, requests[i], string(result))
	}
	for range count {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
}

func TestTimeoutAndLateResponse(t *testing.T) {
	client, _, dec, enc := pipeClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() {
		_, err := client.SendPrompt(ctx, PromptRequest{SessionID: "old", Prompt: []ContentBlock{{Type: "text", Text: "hello"}}})
		finished <- err
	}()
	old := readRequest(t, dec, "session/prompt")
	if err := <-finished; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got %v, want deadline exceeded", err)
	}
	client.mu.Lock()
	pending := len(client.pending)
	client.mu.Unlock()
	if pending != 0 {
		t.Fatalf("timed out request leaked: %d pending", pending)
	}
	// A late reply and an unknown ID must not block delivery of later replies.
	reply(t, enc, old, `{"stopReason":"end_turn"}`)
	reply(t, enc, Request{ID: json.RawMessage(`999`)}, `{"sessionId":"unknown"}`)
	go func() {
		_, err := client.NewSession(testContext(t), NewSessionRequest{CWD: "/workspace"})
		finished <- err
	}()
	req := readRequest(t, dec, "session/new")
	reply(t, enc, req, `{"sessionId":"new"}`)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestTimeoutWhileWriteBlocked(t *testing.T) {
	client, _, _, _ := pipeClient(t)
	// The peer never reads, so the first write blocks. Both callers must time out.
	for range 2 {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		_, err := client.Initialize(ctx, InitializeRequest{})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("got %v, want deadline exceeded", err)
		}
	}
}

func TestCanceledContext(t *testing.T) {
	client, _, _, _ := pipeClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Initialize(ctx, InitializeRequest{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v, want canceled", err)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.nextID != 0 || len(client.pending) != 0 {
		t.Fatal("canceled request was registered")
	}
}

func TestRPCError(t *testing.T) {
	client, _, dec, enc := pipeClient(t)
	finished := make(chan error, 1)
	go func() {
		_, err := client.NewSession(testContext(t), NewSessionRequest{CWD: "/workspace"})
		finished <- err
	}()
	req := readRequest(t, dec, "session/new")
	if err := enc.Encode(Response{JSONRPC: "2.0", ID: req.ID, Error: &Error{Code: -32602, Message: "bad cwd", Data: json.RawMessage(`{"field":"cwd"}`)}}); err != nil {
		t.Fatal(err)
	}
	var rpcErr *Error
	if err := <-finished; !errors.As(err, &rpcErr) || rpcErr.Code != -32602 || !strings.Contains(err.Error(), "bad cwd") || len(rpcErr.Data) == 0 {
		t.Fatalf("unexpected RPC error: %v", err)
	}
}

func TestTransportFailures(t *testing.T) {
	for _, tc := range []struct {
		name string
		wire string
	}{
		{"EOF", ""},
		{"invalid JSON", "{oops}\n"},
		{"invalid version", "{\"jsonrpc\":\"1.0\",\"id\":1,\"result\":{}}\n"},
		{"missing result", "{\"jsonrpc\":\"2.0\",\"id\":1}\n"},
		{"both result and error", "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{},\"error\":{\"code\":1,\"message\":\"oops\"}}\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client, server, dec, _ := pipeClient(t)
			finished := make(chan error, 1)
			go func() {
				_, err := client.NewSession(testContext(t), NewSessionRequest{CWD: "/workspace"})
				finished <- err
			}()
			readRequest(t, dec, "session/new")
			if tc.wire == "" {
				_ = server.Close()
			} else if _, err := io.WriteString(server, tc.wire); err != nil {
				t.Fatal(err)
			}
			if err := <-finished; err == nil || errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("expected immediate transport error, got %v", err)
			}
		})
	}
}

func TestCloseUnblocksPending(t *testing.T) {
	client, _, dec, _ := pipeClient(t)
	ctx := testContext(t)
	finished := make(chan error, 1)
	go func() {
		_, err := client.Initialize(ctx, InitializeRequest{})
		finished <- err
	}()
	readRequest(t, dec, "initialize")
	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			if err := client.Close(); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if err := <-finished; !errors.Is(err, ErrClosed) {
		t.Fatalf("got %v, want closed", err)
	}
	if _, ok := <-client.Notifications(); ok {
		t.Fatal("notification channel still open")
	}
	if _, err := client.Initialize(ctx, InitializeRequest{}); !errors.Is(err, ErrClosed) {
		t.Fatalf("request after Close: %v", err)
	}
}

func TestStdioClient(t *testing.T) {
	stdout, agentWriter := io.Pipe()
	agentReader, stdin := io.Pipe()
	client := NewStdioClient(stdout, stdin)
	t.Cleanup(func() { _ = client.Close() })
	t.Cleanup(func() { _ = agentWriter.Close(); _ = agentReader.Close() })
	finished := make(chan error, 1)
	go func() {
		_, err := client.Initialize(testContext(t), InitializeRequest{})
		finished <- err
	}()
	req := readRequest(t, json.NewDecoder(agentReader), "initialize")
	reply(t, json.NewEncoder(agentWriter), req, `{"protocolVersion":1,"agentCapabilities":{},"authMethods":[]}`)
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestUnsupportedProtocolVersion(t *testing.T) {
	client, _, dec, enc := pipeClient(t)
	finished := make(chan error, 1)
	go func() {
		_, err := client.Initialize(testContext(t), InitializeRequest{})
		finished <- err
	}()
	req := readRequest(t, dec, "initialize")
	reply(t, enc, req, `{"protocolVersion":999,"agentCapabilities":{},"authMethods":[]}`)
	if err := <-finished; err == nil || !strings.Contains(err.Error(), "unsupported protocol version") {
		t.Fatalf("unexpected version error: %v", err)
	}
	select {
	case <-client.done:
	default:
		t.Fatal("connection still open after unsupported version")
	}
}

func TestUnsupportedAgentRequest(t *testing.T) {
	_, _, dec, enc := pipeClient(t)
	if err := enc.Encode(Request{JSONRPC: "2.0", ID: json.RawMessage(`"agent-id"`), Method: "fs/read_text_file"}); err != nil {
		t.Fatal(err)
	}
	var resp Response
	if err := dec.Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if string(resp.ID) != `"agent-id"` || resp.Error == nil || resp.Error.Code != -32601 {
		t.Fatalf("unexpected method rejection: %+v", resp)
	}
}

func TestResponseBeforeEOF(t *testing.T) {
	client, server, dec, enc := pipeClient(t)
	finished := make(chan error, 1)
	go func() {
		resp, err := client.NewSession(testContext(t), NewSessionRequest{CWD: "/workspace"})
		if err == nil && resp.SessionID != "last" {
			err = fmt.Errorf("unexpected final response: %+v", resp)
		}
		finished <- err
	}()
	req := readRequest(t, dec, "session/new")
	reply(t, enc, req, `{"sessionId":"last"}`)
	_ = server.Close()
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}

func TestInvalidResult(t *testing.T) {
	client, _, dec, enc := pipeClient(t)
	finished := make(chan error, 1)
	go func() {
		_, err := client.NewSession(testContext(t), NewSessionRequest{CWD: "/workspace"})
		finished <- err
	}()
	req := readRequest(t, dec, "session/new")
	reply(t, enc, req, `{"sessionId":42}`)
	if err := <-finished; err == nil || !strings.Contains(err.Error(), "decode session/new result") {
		t.Fatalf("expected result decode error, got %v", err)
	}
}

func TestNotificationOverflow(t *testing.T) {
	client, _, _, enc := pipeClient(t)
	for range cap(client.notices) + 1 {
		if err := enc.Encode(Notification{JSONRPC: "2.0", Method: "session/update"}); err != nil {
			t.Fatal(err)
		}
	}
	ctx := testContext(t)
	select {
	case <-client.done:
		if err := client.failure(); err == nil || !strings.Contains(err.Error(), "notification buffer full") {
			t.Fatalf("unexpected overflow error: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestLargeNotification(t *testing.T) {
	client, _, _, enc := pipeClient(t)
	text := strings.Repeat("x", 128*1024)
	params, err := json.Marshal(map[string]any{"text": text})
	if err != nil {
		t.Fatal(err)
	}
	if err := enc.Encode(Notification{JSONRPC: "2.0", Method: "session/update", Params: params}); err != nil {
		t.Fatal(err)
	}
	ctx := testContext(t)
	select {
	case notice := <-client.Notifications():
		if string(notice.Params) != string(params) {
			t.Fatal("large notification was truncated")
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func (failingWriter) Close() error              { return nil }

func TestWriteFailure(t *testing.T) {
	reader, writer := io.Pipe()
	t.Cleanup(func() { _ = writer.Close() })
	client := NewStdioClient(reader, failingWriter{})
	t.Cleanup(func() { _ = client.Close() })
	_, err := client.Initialize(testContext(t), InitializeRequest{})
	if !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("expected write failure, got %v", err)
	}
}

func TestContentBlockJSON(t *testing.T) {
	for _, tc := range []struct {
		name  string
		block ContentBlock
		want  string
	}{
		{"empty text", ContentBlock{Type: "text"}, `{"type":"text","text":""}`},
		{"text", ContentBlock{Type: "text", Text: "hello"}, `{"type":"text","text":"hello"}`},
		{"resource link", ContentBlock{Type: "resource_link", URI: "file:///test", Name: "test"}, `{"type":"resource_link","uri":"file:///test","name":"test"}`},
		{"image", ContentBlock{Type: "image", Data: "aGVsbG8=", MimeType: "image/png"}, `{"type":"image","data":"aGVsbG8=","mimeType":"image/png"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := json.Marshal(tc.block)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
			var roundtrip ContentBlock
			if err := json.Unmarshal(got, &roundtrip); err != nil || roundtrip.Type != tc.block.Type || roundtrip.Text != tc.block.Text {
				t.Fatalf("roundtrip failed: %+v, %v", roundtrip, err)
			}
		})
	}
}
