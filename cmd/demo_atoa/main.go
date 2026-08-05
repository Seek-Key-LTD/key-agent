// Copyright 2026 Google LLC
//
// Agent-to-Agent (A-to-A) Inter-Node Authentication & Invocation Demo
// Demonstrates Node A (Amber) calling Node B (Ruby) over Authentik OAuth2 M2M

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	"google.golang.org/adk/v2/auth"
)

const (
	LLMBaseURL = "http://100.121.16.28:4000/v1"
	LLMAPIKey  = "sk-47318"
	LLMModel   = "nova-deepseek-v4-flash-aggr"
)

// AgentMessage represents an A-to-A invocation request/response payload
type AgentMessage struct {
	Sender    string `json:"sender"`
	Task      string `json:"task"`
	Result    string `json:"result,omitempty"`
	Timestamp string `json:"timestamp"`
}

func main() {
	log.Println("==================================================================")
	log.Println("🚀 Starting Agent-to-Agent (A-to-A) Zero-Trust Call Verification")
	log.Println("==================================================================")

	// Step 1: Start Node B (Ruby) Server on :8089
	serverDone := make(chan bool)
	go startNodeBServer(":8089", serverDone)

	// Wait for Node B to listen
	time.Sleep(500 * time.Millisecond)

	// Step 2: Run Node A (Amber) Client Invocation
	runNodeAClient("http://localhost:8089/agent/invoke")

	log.Println("==================================================================")
	log.Println("🎉 A-to-A Verification Finished Successfully!")
	log.Println("==================================================================")
}

// -----------------------------------------------------------------------------
// Node B (Ruby) - Server Node
// -----------------------------------------------------------------------------
func startNodeBServer(addr string, done chan bool) {
	mux := http.NewServeMux()

	// Endpoints
	mux.HandleFunc("/agent/invoke", authMiddleware(handleAgentInvoke))

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Printf("[Node B (Ruby)] Listening for incoming A-to-A requests on %s...", addr)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[Node B (Ruby)] Server error: %v", err)
	}
}

// Authentik Middleware for Node B (Ruby)
// Validates incoming Bearer token against Authentik UserInfo API
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error": "Missing Authorization header"}`, http.StatusUnauthorized)
			return
		}

		// Validate Token against Authentik UserInfo
		userInfoReq, err := http.NewRequestWithContext(r.Context(), "GET", "https://authentik.capitaltrain.cn/application/o/userinfo/", nil)
		if err != nil {
			http.Error(w, `{"error": "Internal error"}`, http.StatusInternalServerError)
			return
		}
		userInfoReq.Header.Set("Authorization", authHeader)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(userInfoReq)
		if err != nil || resp.StatusCode != http.StatusOK {
			log.Printf("[Node B (Ruby)] ❌ Authentication failed for incoming request: HTTP %d", resp.StatusCode)
			http.Error(w, `{"error": "Invalid or unauthenticated Authentik token"}`, http.StatusUnauthorized)
			return
		}
		defer resp.Body.Close()

		var userInfo struct {
			Sub               string `json:"sub"`
			PreferredUsername string `json:"preferred_username"`
			Name              string `json:"name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
			http.Error(w, `{"error": "Failed to parse UserInfo"}`, http.StatusBadRequest)
			return
		}

		log.Printf("[Node B (Ruby)] 🔒 Authentik Identity Verified! Caller: %s (sub: %s)", userInfo.PreferredUsername, userInfo.Sub[:12])

		// Attach authenticated identity to context
		ctx := context.WithValue(r.Context(), "caller_identity", userInfo.PreferredUsername)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// Handler for Node B (Ruby)
func handleAgentInvoke(w http.ResponseWriter, r *http.Request) {
	caller := r.Context().Value("caller_identity").(string)

	var msg AgentMessage
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("[Node B (Ruby)] Received Task from [%s]: %s", caller, msg.Task)

	// Node B calls LLM (Nova DeepSeek V4) to process task
	llmPrompt := fmt.Sprintf("You are Node B (Ruby), an autonomous Agent in our cluster. Node A (%s) sent you the following task: '%s'. Provide a brief, professional response.", caller, msg.Task)
	llmResponse, err := callLLM(r.Context(), llmPrompt)
	if err != nil {
		log.Printf("[Node B (Ruby)] LLM call failed: %v", err)
		http.Error(w, "LLM processing error", http.StatusInternalServerError)
		return
	}

	respMsg := AgentMessage{
		Sender:    "Node B (Ruby)",
		Task:      msg.Task,
		Result:    llmResponse,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(respMsg)
}

// -----------------------------------------------------------------------------
// Node A (Amber) - Client Node
// -----------------------------------------------------------------------------
func runNodeAClient(targetURL string) {
	log.Println("[Node A (Amber)] Initializing Authentik OAuth2 Client Credentials...")

	// Amber Credentials (retrieved from Vault secret/data/dev/authentik/amber)
	cfg := auth.AuthentikConfig{
		BaseURL:      "https://authentik.capitaltrain.cn",
		ClientID:     "agent-client-amber",
		ClientSecret: "Irmvd0LUEObYeETi3mM3Y4m20rPtPBlKn9EupkZR8qVWT9o3WJQtnkBVlMKJkqz3QqzjVNmT1p4jCfHoPbu0N7lQts9QHmPwQ14XYNK6pYbgYeVXqkMfnr8bZ0bHf9lQ",
		Scopes:       []string{"openid", "profile", "email"},
	}

	provider := auth.AuthentikClientCredentials(cfg)
	ctx := context.Background()

	cred, err := provider.Credential(ctx)
	if err != nil {
		log.Fatalf("[Node A (Amber)] Failed to obtain Authentik Credential: %v", err)
	}

	reqPayload := AgentMessage{
		Sender:    "Node A (Amber)",
		Task:      "Please summarize our cluster deployment status and verify A-to-A connectivity.",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	bodyBytes, _ := json.Marshal(reqPayload)

	req, err := http.NewRequestWithContext(ctx, "POST", targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		log.Fatalf("[Node A (Amber)] Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Apply Authentik OAuth2 Credential (Injects Authorization: Bearer <token>)
	if err := cred.Apply(req.Header); err != nil {
		log.Fatalf("[Node A (Amber)] Failed to apply Authentik credential: %v", err)
	}

	log.Printf("[Node A (Amber)] Sending A-to-A HTTP request with Bearer token to Node B...")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("[Node A (Amber)] Invocation failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		log.Fatalf("[Node A (Amber)] HTTP Error from Node B: %d - %s", resp.StatusCode, string(respBody))
	}

	var respMsg AgentMessage
	if err := json.NewDecoder(resp.Body).Decode(&respMsg); err != nil {
		log.Fatalf("[Node A (Amber)] Failed to decode response: %v", err)
	}

	log.Printf("[Node A (Amber)] Response received from [%s]:\n\"%s\"", respMsg.Sender, respMsg.Result)
}

// Helper to call OpenAI-compatible LLM endpoint
func callLLM(ctx context.Context, prompt string) (string, error) {
	reqBody := map[string]interface{}{
		"model": LLMModel,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	jsonBytes, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", LLMBaseURL+"/chat/completions", bytes.NewReader(jsonBytes))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+LLMAPIKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LLM API returned %d: %s", resp.StatusCode, string(b))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.Choices) > 0 {
		return result.Choices[0].Message.Content, nil
	}
	return "No response generated", nil
}
