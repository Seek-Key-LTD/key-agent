// Copyright 2026 Google LLC
//
// Master Orchestrator Demo: Multi-Agent Interactive Execution & Feishu Calendar Lock
// Master -> Node A (Amber) -> Node B (Ruby) -> Python Execution -> Response

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
	"google.golang.org/adk/v2/integration"
	"google.golang.org/adk/v2/messaging"
)

type AgentTask struct {
	Sender    string `json:"sender"`
	Question  string `json:"question"`
	Timestamp string `json:"timestamp"`
}

type AgentReply struct {
	Responder     string `json:"responder"`
	PythonCode    string `json:"python_code"`
	ExecOutput    string `json:"exec_output"`
	Result        string `json:"result"`
	CalendarLock  string `json:"calendar_lock"`
	MatrixEvtID   string `json:"matrix_evt_id"`
}

func main() {
	log.Println("==================================================================")
	log.Println("👑 MASTER ORCHESTRATOR: Initiating 2-Node Live A-to-A Task Execution")
	log.Println("==================================================================")

	// Step 1: Start Node B (Ruby) Agent Server on :8091
	go startNodeB()
	time.Sleep(500 * time.Millisecond)

	// Step 2: Master instructs Node A (Amber) to trigger Node B
	log.Println("👑 [Master] Commanding Node A (Amber) to ask Node B (Ruby): 'What is 1+1?'")
	runNodeA()

	log.Println("==================================================================")
	log.Println("🎉 MASTER ORCHESTRATOR: Live Multi-Agent Execution Completed!")
	log.Println("==================================================================")
}

// -----------------------------------------------------------------------------
// Node B (Ruby) Server
// -----------------------------------------------------------------------------
func startNodeB() {
	mux := http.NewServeMux()
	mux.HandleFunc("/agent/invoke", authMiddleware(handleNodeBTask))

	log.Println("[Node B (Ruby)] Server online on :8091 (A-to-A Receiver)...")
	if err := http.ListenAndServe(":8091", mux); err != nil {
		log.Fatalf("[Node B (Ruby)] Error: %v", err)
	}
}

func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, `{"error": "Missing Token"}`, http.StatusUnauthorized)
			return
		}

		// Authentik UserInfo validation
		userInfoReq, _ := http.NewRequestWithContext(r.Context(), "GET", "https://authentik.capitaltrain.cn/application/o/userinfo/", nil)
		userInfoReq.Header.Set("Authorization", authHeader)

		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Do(userInfoReq)
		if err != nil || resp.StatusCode != http.StatusOK {
			http.Error(w, `{"error": "Authentik validation failed"}`, http.StatusUnauthorized)
			return
		}
		defer resp.Body.Close()

		var info struct {
			PreferredUsername string `json:"preferred_username"`
		}
		json.NewDecoder(resp.Body).Decode(&info)

		ctx := context.WithValue(r.Context(), "caller", info.PreferredUsername)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func handleNodeBTask(w http.ResponseWriter, r *http.Request) {
	caller := r.Context().Value("caller").(string)

	var task AgentTask
	json.NewDecoder(r.Body).Decode(&task)

	log.Printf("[Node B (Ruby)] 🔒 Authentik Verified Caller [%s]", caller)
	log.Printf("[Node B (Ruby)] Received Question from [%s]: '%s'", task.Sender, task.Question)

	// 1. Lock Feishu Calendar Occupation (Estimated 30 mins / 0.5h)
	duration := 30 * time.Minute
	occRes, _ := integration.CreateOccupationEvent(r.Context(), integration.OccupationRequest{
		AgentName: "ruby",
		TaskTitle: "Execute Python Script for 1+1 Calculation",
		IssueURL:  "http://master.local/task/calc-101",
		Duration:  duration,
		StartTime: time.Now(),
	})

	// 2. Generate Python Script dynamically
	pyCode := `import sys
val1 = 1
val2 = 1
res = val1 + val2
print(f"Calculated: {val1} + {val2} = {res}")
`
	scriptPath := "/tmp/calc_one_plus_one.py"
	os.WriteFile(scriptPath, []byte(pyCode), 0755)

	// 3. Execute Python Script
	log.Printf("[Node B (Ruby)] 🐍 Writing and executing Python script '%s'...", scriptPath)
	cmd := exec.Command("python3", scriptPath)
	outBytes, err := cmd.CombinedOutput()
	outStr := string(outBytes)
	if err != nil {
		outStr = fmt.Sprintf("Error: %v, Output: %s", err, outStr)
	}

	log.Printf("[Node B (Ruby)] 💻 Python Output: %s", outStr)

	// 4. Matrix Notification & Reaction
	matrixClient := messaging.NewMatrixClient(messaging.MatrixConfig{
		HomeserverURL: "https://matrix.capitaltrain.cn",
		AccessToken:   "syt_bot_token",
	})
	evtID, _ := matrixClient.SendMessage(r.Context(), "", fmt.Sprintf("Node B (Ruby) executed Python script for [%s]. Result: 1+1=2", caller))
	matrixClient.SendReaction(r.Context(), "", evtID, "✅")

	reply := AgentReply{
		Responder:    "Node B (Ruby)",
		PythonCode:   pyCode,
		ExecOutput:   outStr,
		Result:       "1 + 1 = 2",
		CalendarLock: fmt.Sprintf("Locked 30m (%s ~ %s)", occRes.StartTime.Format("15:04"), occRes.EndTime.Format("15:04")),
		MatrixEvtID:  evtID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

// -----------------------------------------------------------------------------
// Node A (Amber) Client
// -----------------------------------------------------------------------------
func runNodeA() {
	log.Println("[Node A (Amber)] Lock Feishu Calendar Occupation (Est: 30m)...")
	ctx := context.Background()

	_, _ = integration.CreateOccupationEvent(ctx, integration.OccupationRequest{
		AgentName: "amber",
		TaskTitle: "Delegate 1+1 Task to Node B (Ruby)",
		IssueURL:  "http://master.local/task/calc-101",
		Duration:  30 * time.Minute,
		StartTime: time.Now(),
	})

	// Get Authentik OAuth2 Access Token for Node A
	log.Println("[Node A (Amber)] Obtaining Authentik OAuth2 Bearer Token...")
	provider := auth.AuthentikClientCredentials(auth.AuthentikConfig{
		BaseURL:      "https://authentik.capitaltrain.cn",
		ClientID:     "agent-client-amber",
		ClientSecret: "Irmvd0LUEObYeETi3mM3Y4m20rPtPBlKn9EupkZR8qVWT9o3WJQtnkBVlMKJkqz3QqzjVNmT1p4jCfHoPbu0N7lQts9QHmPwQ14XYNK6pYbgYeVXqkMfnr8bZ0bHf9lQ",
		Scopes:       []string{"openid", "profile", "email"},
	})

	cred, err := provider.Credential(ctx)
	if err != nil {
		log.Fatalf("[Node A (Amber)] Token error: %v", err)
	}

	taskPayload := AgentTask{
		Sender:    "Node A (Amber)",
		Question:  "What is 1+1? Please write a Python script to calculate it, run it, and reply.",
		Timestamp: time.Now().Format(time.RFC3339),
	}
	bodyBytes, _ := json.Marshal(taskPayload)

	req, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost:8091/agent/invoke", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	cred.Apply(req.Header)

	log.Println("[Node A (Amber)] Sending A-to-A Task Request with Bearer Token to Node B (Ruby)...")
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatalf("[Node A (Amber)] Invocation error: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		log.Fatalf("[Node A (Amber)] Node B returned error %d: %s", resp.StatusCode, string(b))
	}

	var reply AgentReply
	json.NewDecoder(resp.Body).Decode(&reply)

	log.Println("==================================================================")
	log.Printf("📥 [Node A (Amber)] Verified Reply Received from [%s]:", reply.Responder)
	log.Printf("   - Calendar Lock Status: %s", reply.CalendarLock)
	log.Printf("   - Executed Python Code:\n%s", reply.PythonCode)
	log.Printf("   - Python Exec Output:   %s", reply.ExecOutput)
	log.Printf("   - Final Answer:         %s", reply.Result)
	log.Println("==================================================================")
}
