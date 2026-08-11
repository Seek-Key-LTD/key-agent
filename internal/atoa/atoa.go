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
	"os"
	"strings"
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
// ExtendedAgentCard 扩展 A2A 标准 AgentCard，附加 ASN 自定义字段。
// 内嵌标准 AgentCard 输出标准字段，再附加 soul_name/evm_address/constellation/coherence。
type ExtendedAgentCard struct {
	*a2a.AgentCard
	SoulName      string            `json:"soul_name,omitempty"`
	EVMAddress    string            `json:"evm_address,omitempty"`
	Constellation string            `json:"constellation,omitempty"`
	Coherence     map[string]string `json:"coherence,omitempty"`
}

// NewStaticExtendedAgentCardHandler 输出扩展 AgentCard（含 ASN 自定义字段）。
func NewStaticExtendedAgentCardHandler(card *ExtendedAgentCard) http.Handler {
	bytes, err := json.Marshal(card)
	if err != nil {
		panic(err.Error())
	}
	return http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		if req.Method == "OPTIONS" {
			rw.Header().Set("Allow", "GET, OPTIONS")
			rw.WriteHeader(http.StatusOK)
			return
		}
		if req.Method != "GET" {
			rw.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		rw.Header().Set("Content-Type", "application/json")
		rw.Header().Set("Access-Control-Allow-Origin", "*")
		rw.Write(bytes)
	})
}

// LoadASNCardFields 从 Vault 读取 agentcard secret（soul_name/evm_address/constellation_house），
// 并从 Consul KV 读取 coherence（其他 agent 对本 agent 的称呼昵称）。
// 任一来源失败时降级为空值，不阻塞 AgentCard 输出。
func LoadASNCardFields(vaultAddr, agentName string) (soulName, evmAddr, constellation string, coherence map[string]string) {
	coherence = map[string]string{}
	if agentName == "" {
		return
	}

	// 1. Vault: secret/dev/agentcard/<agentName>
	if vaultAddr != "" {
		client := &http.Client{Timeout: 5 * time.Second}
		token := readVaultToken()
		if token != "" {
			req, err := http.NewRequest("GET", vaultAddr+"/v1/secret/data/dev/agentcard/"+agentName, nil)
			if err == nil {
				req.Header.Set("X-Vault-Token", token)
				if resp, err := client.Do(req); err == nil {
					defer resp.Body.Close()
					var vr struct {
						Data struct {
							Data struct {
								SoulName      string `json:"soul_name"`
								EVMAddress    string `json:"evm_address"`
								Constellation string `json:"constellation_house"`
							} `json:"data"`
						} `json:"data"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&vr); err == nil {
						soulName = vr.Data.Data.SoulName
						evmAddr = vr.Data.Data.EVMAddress
						constellation = vr.Data.Data.Constellation
					}
				}
			}
		}
	}

	// 2. Consul KV: coherence/<from>/<agentName> — 收集所有 agent 对本 agent 的昵称
	consulAddr := os.Getenv("CONSUL_ADDR")
	if consulAddr == "" {
		consulAddr = "http://127.0.0.1:8500"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	if resp, err := client.Get(consulAddr + "/v1/kv/coherence/?recurse&keys"); err == nil {
		defer resp.Body.Close()
		var keys []string
		if err := json.NewDecoder(resp.Body).Decode(&keys); err == nil {
			for _, k := range keys {
				// key 格式: coherence/<from>/<agentName>
				parts := strings.Split(k, "/")
				if len(parts) == 3 && parts[0] == "coherence" && parts[2] == agentName {
					if vresp, err := client.Get(consulAddr + "/v1/kv/" + k + "?raw"); err == nil {
						var rel struct {
							Nickname string `json:"nickname"`
						}
						_ = json.NewDecoder(vresp.Body).Decode(&rel)
						vresp.Body.Close()
						if rel.Nickname != "" {
							coherence[parts[1]] = rel.Nickname
						}
					}
				}
			}
		}
	}

	return
}

// readVaultToken 依次尝试常见 token 来源。
func readVaultToken() string {
	paths := []string{"/etc/vault-agent/token", "/root/.vault-token"}
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return os.Getenv("VAULT_TOKEN")
}

// agentName/description 为名片字段；userInfoURL 为 authentik userinfo 端点。
// soulName/evmAddress/constellation/coherence 为 ASN 自定义字段（花神/钱包/星宫/关系昵称）。
func Mount(mux *http.ServeMux, agentName, agentDesc, agentAddr, userInfoURL string, llm LLMConfig,
	soulName, evmAddress, constellation string, coherence map[string]string) {
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

	extCard := &ExtendedAgentCard{
		AgentCard:     card,
		SoulName:      soulName,
		EVMAddress:    evmAddress,
		Constellation: constellation,
		Coherence:     coherence,
	}

	handler := a2asrv.NewJSONRPCHandler(
		a2asrv.NewHandler(&AgentExecutor{LLM: llm}, a2asrv.WithExtendedAgentCard(card)),
	)

	mux.Handle("/a2a", auth.AuthentikUserInfoMiddleware(userInfoURL, handler))
	mux.Handle("/.well-known/agent-card.json", NewStaticExtendedAgentCardHandler(extCard))
}
