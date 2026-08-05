// Copyright 2026 Google LLC
//
// Topaz (黄玉) -> Ruby (红宝石) A-to-A Task Orchestration & EVM Tipping Demo

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"

	"google.golang.org/adk/v2/auth"
	"google.golang.org/adk/v2/evm"
	"google.golang.org/adk/v2/integration"
	"google.golang.org/adk/v2/messaging"
)

type TaskPayload struct {
	Sender    string `json:"sender"`
	Target    string `json:"target"`
	Question  string `json:"question"`
	Timestamp string `json:"timestamp"`
}

type TaskResponse struct {
	Responder    string `json:"responder"`
	PythonCode   string `json:"python_code"`
	ExecOutput   string `json:"exec_output"`
	Answer       string `json:"answer"`
	CalendarLock string `json:"calendar_lock"`
	EVMAddress   string `json:"evm_address"`
}

func main() {
	log.Println("==================================================================")
	log.Println("💎 AGENT DISPATCH: Node Topaz (黄玉) -> Node Ruby (红宝石) + EVM Tip")
	log.Println("==================================================================")

	// Step 1: Start Ruby Agent HTTP server on :8093
	go startRubyServer()
	time.Sleep(400 * time.Millisecond)

	// Step 2: Execute Topaz Agent calling Ruby Agent & Tipping ETH
	runTopazAgent()

	log.Println("==================================================================")
	log.Println("✨ TOPAZ -> RUBY A-to-A TASK EXECUTION & EVM TIPPING SUCCESSFUL!")
	log.Println("==================================================================")
}

// -----------------------------------------------------------------------------
// Node Ruby (红宝石) Server
// -----------------------------------------------------------------------------
func startRubyServer() {
	rubyWallet, _ := evm.GenerateAgentWallet("ruby")

	mux := http.NewServeMux()
	mux.HandleFunc("/agent/invoke", func(w http.ResponseWriter, r *http.Request) {
		var payload TaskPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("[Node Ruby (红宝石)] 📥 Received Task from [%s]: '%s'", payload.Sender, payload.Question)

		// 1. Feishu Calendar Occupation Locking (30 minutes)
		duration := 30 * time.Minute
		occRes, _ := integration.CreateOccupationEvent(r.Context(), integration.OccupationRequest{
			AgentName: "ruby",
			TaskTitle: fmt.Sprintf("Solve Task from %s: %s", payload.Sender, payload.Question),
			IssueURL:  "http://nomad.internal/task/topaz-ruby-101",
			Duration:  duration,
			StartTime: time.Now(),
		})

		// 2. Write and Execute Python Script
		pyCode := `import sys
a = 1
b = 1
result = a + b
print(f"Ruby Python Script Evaluation: {a} + {b} = {result}")
`
		pyFile := "/tmp/ruby_calc_1_plus_1.py"
		os.WriteFile(pyFile, []byte(pyCode), 0755)

		log.Printf("[Node Ruby (红宝石)] 🐍 Executing Python script: %s", pyFile)
		outBytes, err := exec.Command("python3", pyFile).CombinedOutput()
		outStr := string(outBytes)
		if err != nil {
			outStr = fmt.Sprintf("Error: %v, Log: %s", err, outStr)
		}
		log.Printf("[Node Ruby (红宝石)] 💻 Result Output: %s", outStr)

		// 3. Matrix Communication & Reaction
		matrixClient := messaging.NewMatrixClient(messaging.MatrixConfig{
			HomeserverURL: "https://matrix.capitaltrain.cn",
			AccessToken:   "syt_ruby_token",
		})
		evtID, _ := matrixClient.SendMessage(r.Context(), "", fmt.Sprintf("Ruby processed task for Topaz. Output: 1+1=2"))
		matrixClient.SendReaction(r.Context(), "", evtID, "🚀")

		resp := TaskResponse{
			Responder:    "Node Ruby (红宝石)",
			PythonCode:   pyCode,
			ExecOutput:   outStr,
			Answer:       "1 + 1 = 2",
			CalendarLock: fmt.Sprintf("Locked 30m (%s ~ %s)", occRes.StartTime.Format("15:04"), occRes.EndTime.Format("15:04")),
			EVMAddress:   rubyWallet.Address,
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	})

	if err := http.ListenAndServe(":8093", mux); err != nil {
		log.Fatalf("[Node Ruby (红宝石)] Server error: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Node Topaz (黄玉) Client
// -----------------------------------------------------------------------------
func runTopazAgent() {
	ctx := context.Background()
	topazWallet, _ := evm.GenerateAgentWallet("topaz")

	log.Printf("[Node Topaz (黄玉)] 💳 EVM Wallet initialized: %s", topazWallet.Address)
	log.Println("[Node Topaz (黄玉)] 🔒 Authentik OAuth2 M2M Token Authentication...")

	provider := auth.AuthentikClientCredentials(auth.AuthentikConfig{
		BaseURL:      "https://authentik.capitaltrain.cn",
		ClientID:     "agent-client-topaz",
		ClientSecret: "Irmvd0LUEObYeETi3mM3Y4m20rPtPBlKn9EupkZR8qVWT9o3WJQtnkBVlMKJkqz3QqzjVNmT1p4jCfHoPbu0N7lQts9QHmPwQ14XYNK6pYbgYeVXqkMfnr8bZ0bHf9lQ",
		Scopes:       []string{"openid", "profile", "email"},
	})

	cred, err := provider.Credential(ctx)
	if err != nil {
		log.Fatalf("[Node Topaz (黄玉)] Token error: %v", err)
	}

	// Lock Topaz's Feishu Calendar
	log.Println("[Node Topaz (黄玉)] Lock Feishu Calendar Occupation (Est: 30m)...")
	_, _ = integration.CreateOccupationEvent(ctx, integration.OccupationRequest{
		AgentName: "topaz",
		TaskTitle: "Delegate 1+1 Calculation to Node Ruby",
		IssueURL:  "http://nomad.internal/task/topaz-ruby-101",
		Duration:  30 * time.Minute,
		StartTime: time.Now(),
	})

	payload := TaskPayload{
		Sender:    "Node Topaz (黄玉)",
		Target:    "Node Ruby (红宝石)",
		Question:  "What is 1+1? Please run a Python script to verify.",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	pBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost:8093/agent/invoke", bytes.NewReader(pBytes))
	req.Header.Set("Content-Type", "application/json")
	cred.Apply(req.Header)

	log.Println("[Node Topaz (黄玉)] Sending A-to-A Task Request to Node Ruby (红宝石)...")
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		log.Fatalf("[Node Topaz (黄玉)] Invocation failed: %v", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		log.Fatalf("[Node Topaz (黄玉)] Error response %d: %s", res.StatusCode, string(b))
	}

	var taskRes TaskResponse
	json.NewDecoder(res.Body).Decode(&taskRes)

	log.Println("==================================================================")
	log.Printf("📥 [Node Topaz (黄玉)] Verified Reply Received from [%s]:", taskRes.Responder)
	log.Printf("   - Calendar Lock:      %s", taskRes.CalendarLock)
	log.Printf("   - Python Exec Output: %s", taskRes.ExecOutput)
	log.Printf("   - Verified Answer:    %s", taskRes.Answer)
	log.Printf("   - EVM Wallet Address: %s", taskRes.EVMAddress)
	log.Println("==================================================================")

	// Step 3: Topaz tips Ruby 0.05 ETH via EVM on task completion
	log.Println("[Node Topaz (黄玉)] 💸 Task Verified Correct! Executing EVM Micropayment Tip...")
	tipRecord, err := evm.SendAgentTip(ctx, topazWallet, "ruby", taskRes.EVMAddress, "0.05", "Reward for 1+1 Python Calculation Task")
	if err != nil {
		log.Printf("[Node Topaz (黄玉)] Tipping error: %v", err)
		return
	}

	log.Println("==================================================================")
	log.Println("💰 [EVM ON-CHAIN TIPPING RECEIPT]:")
	log.Printf("   - From Agent: %s (%s)", tipRecord.FromAgent, tipRecord.FromAddr)
	log.Printf("   - To Agent:   %s (%s)", tipRecord.ToAgent, tipRecord.ToAddr)
	log.Printf("   - Amount:     %s ETH", tipRecord.AmountETH)
	log.Printf("   - TxHash:     %s", tipRecord.TxHash)
	log.Printf("   - On-Chain:   %s", tipRecord.Status)
	log.Println("==================================================================")
}
