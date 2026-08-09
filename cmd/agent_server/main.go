// A2A Agent Server — 正式版
// 每个 agent 起一个 A2A HTTP 端点 + AgentCard 名片端点。
// 凭据/名片从 Vault 渲染的 env 读取（不 hardcode）。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"google.golang.org/adk/v2/auth"
	"github.com/a2aproject/a2a-go/v2/a2asrv"
)

// AgentExecutor 实现 a2asrv.AgentExecutor：处理 A2A 任务
type AgentExecutor struct {
	LLMBaseURL string
	LLMAPIKey  string
	LLMModel   string
}

func (e *AgentExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	msg := execCtx.Message
	taskText := ""
	if msg != nil {
		for _, p := range msg.Parts {
			taskText += p.Text()
		}
	}
	return func(yield func(a2a.Event, error) bool) {
		resp, err := callLLM(ctx, e, taskText)
		if err != nil {
			yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed, nil), nil)
			yield(nil, err)
			return
		}
		yield(a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(resp)), nil)
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}

func (e *AgentExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func callLLM(ctx context.Context, e *AgentExecutor, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":    e.LLMModel,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", e.LLMBaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.LLMAPIKey)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

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
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("LLM returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}

func main() {
	baseURL := os.Getenv("LLM_BASE_URL")
	apiKey := os.Getenv("LLM_API_KEY")
	model := os.Getenv("LLM_MODEL")
	addr := os.Getenv("AGENT_ADDR")
	if addr == "" {
		addr = ":18791"
	}
	agentName := os.Getenv("AGENT_NAME")
	if agentName == "" {
		agentName = "unknown"
	}
	agentDesc := os.Getenv("AGENT_DESCRIPTION")
	agentSkills := os.Getenv("AGENT_SKILLS")
	userInfoURL := os.Getenv("AUTHENTIK_USERINFO_URL")
	if userInfoURL == "" {
		userInfoURL = "https://authentik.capitaltrain.cn/application/o/userinfo/"
	}

	executor := &AgentExecutor{LLMBaseURL: baseURL, LLMAPIKey: apiKey, LLMModel: model}

	// AgentCard（最小版，后续从 Vault 名片数据渲染）
	card := &a2a.AgentCard{
		Name:        agentName,
		Description: agentDesc,
		Version:     "0.1.0",
		SupportedInterfaces: []*a2a.AgentInterface{{
			URL:             "http://" + addr + "/a2a",
			ProtocolBinding: a2a.TransportProtocolJSONRPC,
		}},
		Skills: []a2a.AgentSkill{{Description: agentSkills}},
	}

	// A2A JSON-RPC handler
	handler := a2asrv.NewJSONRPCHandler(
		a2asrv.NewHandler(executor, a2asrv.WithExtendedAgentCard(card)),
	)

	// HTTP mux
	mux := http.NewServeMux()
	mux.Handle("/a2a", auth.AuthentikUserInfoMiddleware(userInfoURL, handler))
	mux.Handle("/.well-known/agent-card.json", a2asrv.NewStaticAgentCardHandler(card))

	log.Printf("[%s] A2A Agent Server listening on %s (model=%s)", agentName, addr, model)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
