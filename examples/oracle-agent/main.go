// oracle-agent is a real agent: Oracle memory + LiteLLM chat/completions.
// It reads documents, sends them to an LLM via standard OpenAI chat API,
// and stores results in Oracle ADB.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	memOracle "google.golang.org/adk/v2/memory/oracle"
	sessOracle "google.golang.org/adk/v2/session/oracle"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
}

type chatResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func main() {
	dsn := flag.String("dsn", os.Getenv("ORACLE_DSN"), "Oracle DSN")
	agentName := flag.String("agent", "oracle-agent", "Agent name")
	memoryKey := flag.String("memory-key", os.Getenv("ORACLE_MEMORY_KEY"), "AES-256 key hex")
	baseURL := flag.String("base-url", os.Getenv("OPENAI_BASE_URL"), "LLM base URL")
	apiKey := flag.String("secret", os.Getenv("LITELLM_API_KEY"), "LLM API key")
	modelID := flag.String("model", "interim-deepseek-v4-flash", "Model ID")
	prompt := flag.String("prompt", "", "Prompt text")
	promptFile := flag.String("prompt-file", "", "Read prompt from file")
	instruction := flag.String("instruction", "你是一个严谨的历史文本分析助手。根据用户提供的原文和分析材料，给出详细、有判断力的评价。", "System instruction")
	flag.Parse()

	if *baseURL == "" || *apiKey == "" {
		log.Fatal("-base-url and -secret required")
	}

	var userMessage string
	if *promptFile != "" {
		data, err := os.ReadFile(*promptFile)
		if err != nil {
			log.Fatalf("read prompt file: %v", err)
		}
		userMessage = string(data)
	} else if *prompt != "" {
		userMessage = *prompt
	} else {
		log.Fatal("provide -prompt or -prompt-file")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	// 1. Connect Oracle memory (for future use)
	if *dsn != "" {
		mem := memOracle.NewOracleMemory(*dsn, *memoryKey)
		if err := mem.Connect(ctx); err != nil {
			log.Printf("warning: memory connect failed: %v (continuing without Oracle memory)", err)
		} else {
			log.Printf("Oracle memory connected")
			defer mem.Close()
		}

		sessSvc := sessOracle.NewOracleSession(*dsn)
		if err := sessSvc.Connect(ctx); err != nil {
			log.Printf("warning: session connect failed: %v", err)
		} else {
			defer sessSvc.Close()
		}
	}

	// 2. Call LLM via standard OpenAI chat/completions API
	reqBody := chatRequest{
		Model: *modelID,
		Messages: []chatMessage{
			{Role: "system", Content: *instruction},
			{Role: "user", Content: userMessage},
		},
	}

	jsonBody, _ := json.Marshal(reqBody)
	url := fmt.Sprintf("%s/chat/completions", *baseURL)

	log.Printf("Agent %q | model=%s | prompt=%d chars | calling LLM...", *agentName, *modelID, len(userMessage))

	req, _ := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+*apiKey)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("LLM call failed: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		log.Fatalf("LLM error %d: %s", resp.StatusCode, string(body))
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		log.Fatalf("parse response: %v", err)
	}

	if chatResp.Error != nil {
		log.Fatalf("LLM error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		log.Fatal("no choices in response")
	}

	// 3. Output result
	fmt.Println(chatResp.Choices[0].Message.Content)
}
