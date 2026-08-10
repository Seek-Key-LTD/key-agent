// Package atoa 提供正式的 A2A (Agent-to-Agent) 服务端组件。
// 供 keyagent 单体进程复用：executor + AgentCard + HTTP handler 装配。
package atoa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"google.golang.org/adk/v2/auth"
)

// LLMConfig 配置 LLM 调用（litellm）。
type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

// AgentExecutor 实现 a2asrv.AgentExecutor。
type AgentExecutor struct {
	LLM LLMConfig
}

// Execute 处理 A2A message，调 LLM 生成回复。
func (e *AgentExecutor) Execute(ctx context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	taskText := ""
	if msg := execCtx.Message; msg != nil {
		for _, p := range msg.Parts {
			taskText += p.Text()
		}
	}
	return func(yield func(a2a.Event, error) bool) {
		resp, err := callLLM(ctx, e.LLM, taskText)
		if err != nil {
			yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateFailed, nil), nil)
			yield(nil, err)
			return
		}
		yield(a2a.NewMessage(a2a.MessageRoleAgent, a2a.NewTextPart(resp)), nil)
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCompleted, nil), nil)
	}
}

// Cancel 取消任务。
func (e *AgentExecutor) Cancel(_ context.Context, execCtx *a2asrv.ExecutorContext) iter.Seq2[a2a.Event, error] {
	return func(yield func(a2a.Event, error) bool) {
		yield(a2a.NewStatusUpdateEvent(execCtx, a2a.TaskStateCanceled, nil), nil)
	}
}

func callLLM(ctx context.Context, llm LLMConfig, prompt string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"model":    llm.Model,
		"messages": []map[string]string{{"role": "user", "content": prompt}},
	})
	req, err := http.NewRequestWithContext(ctx, "POST", llm.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+llm.APIKey)

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

// Mount 把 A2A handler + AgentCard 端点挂到 mux 上。
// agentName/description 为名片字段；userInfoURL 为 authentik userinfo 端点。
func Mount(mux *http.ServeMux, agentName, agentDesc, agentAddr, userInfoURL string, llm LLMConfig) {
	card := &a2a.AgentCard{
		Name:        agentName,
		Description: agentDesc,
		Version:     "0.1.0",
		SupportedInterfaces: []*a2a.AgentInterface{{
			URL:             "http://" + agentAddr + "/a2a",
			ProtocolBinding: a2a.TransportProtocolJSONRPC,
		}},
		Skills: []a2a.AgentSkill{{Description: agentDesc}},
	}

	handler := a2asrv.NewJSONRPCHandler(
		a2asrv.NewHandler(&AgentExecutor{LLM: llm}, a2asrv.WithExtendedAgentCard(card)),
	)

	mux.Handle("/a2a", auth.AuthentikUserInfoMiddleware(userInfoURL, handler))
	mux.Handle("/.well-known/agent-card.json", a2asrv.NewStaticAgentCardHandler(card))
}
