// Package atoa 提供正式的 A2A (Agent-to-Agent) 服务端组件。
// 供 keyagent 单体进程复用：executor + AgentCard + HTTP handler 装配。
package atoa

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2asrv"

	"google.golang.org/adk/v2/auth"
	"google.golang.org/adk/v2/internal/secrets"
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

// LoadASNCardFields 从 Authentik (主权真相源优先)、凭据面 (secrets Provider) 或环境读取 agentcard 属性（soul_name/evm_address/constellation_house），
// 并从 Consul KV 读取 coherence（其他 agent 对本 agent 的称呼昵称，仅 host 形态）。
// 任一来源失败时平滑降级，不阻塞 AgentCard 输出。
func LoadASNCardFields(agentName string) (soulName, evmAddr, constellation string, coherence map[string]string) {
	coherence = map[string]string{}
	if agentName == "" {
		return
	}

	// 0. Environment variables (最高优先级显式覆盖)
	if v := os.Getenv("SOUL_NAME"); v != "" {
		soulName = v
	}
	if v := os.Getenv("EVM_ADDRESS"); v != "" {
		evmAddr = v
	}
	if v := os.Getenv("CONSTELLATION"); v != "" {
		constellation = v
	}

	// 1. Authentik Identity Connector (SSOT): 连接 Authentik 纯管道自省器
	if soulName == "" || evmAddr == "" || constellation == "" {
		authentikURL := os.Getenv("AUTHENTIK_URL")
		if authentikURL == "" {
			authentikURL = "https://authentik.capitaltrain.cn"
		}
		authentikToken := readAuthentikToken()
		if authentikToken != "" {
			client := &http.Client{Timeout: 5 * time.Second}
			searchURL := fmt.Sprintf("%s/api/v3/core/users/?search=%s", authentikURL, agentName)
			req, err := http.NewRequest("GET", searchURL, nil)
			if err == nil {
				req.Header.Set("Authorization", "Bearer "+authentikToken)
				if resp, err := client.Do(req); err == nil {
					defer resp.Body.Close()
					var ur struct {
						Results []struct {
							Username   string `json:"username"`
							Attributes struct {
								SoulName      string `json:"soul_name"`
								EVMAddress    string `json:"evm_address"`
								Constellation string `json:"constellation_house"`
								Seat          string `json:"seat"`
							} `json:"attributes"`
						} `json:"results"`
					}
					if err := json.NewDecoder(resp.Body).Decode(&ur); err == nil {
						for _, r := range ur.Results {
							if r.Username == "agent-sa-"+agentName || r.Attributes.Seat == agentName || r.Username == agentName {
								if soulName == "" && r.Attributes.SoulName != "" {
									soulName = r.Attributes.SoulName
								}
								if evmAddr == "" && r.Attributes.EVMAddress != "" {
									evmAddr = r.Attributes.EVMAddress
								}
								if constellation == "" && r.Attributes.Constellation != "" {
									constellation = r.Attributes.Constellation
								}
								log.Printf("[WhoAmI] Identified as %s via Authentik Connector (Soul: %s, House: %s, EVM: %s)",
									agentName, soulName, constellation, evmAddr)
								break
							}
						}
					}
				}
			}
		}
	}

	// 2. 凭据面降级读取: <mount>/dev/agentcard/<agentName> (Host=Vault / Guest=Infisical)
	if soulName == "" || evmAddr == "" || constellation == "" {
		if p, err := secrets.ForFlavor(); err == nil {
			if m, err := p.Get(context.Background(), "dev/agentcard/"+agentName); err == nil {
				if soulName == "" {
					soulName = m["soul_name"]
				}
				if evmAddr == "" {
					evmAddr = m["evm_address"]
				}
				if constellation == "" {
					constellation = m["constellation_house"]
				}
			}
		} else if secrets.Flavor() == secrets.FlavorGuest {
			log.Printf("atoa: guest secret provider unavailable: %v (agentcard degraded to empty)", err)
		}
	}

	// 3. Consul KV: coherence/<from>/<agentName> — 收集所有 agent 对本 agent 的昵称（host-only）
	if secrets.Flavor() == secrets.FlavorGuest {
		return
	}
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

// readAuthentikToken 获取叩门 Authentik 的凭据 (环境变量、本地文件、或从 secrets Provider 读取)。
func readAuthentikToken() string {
	if t := os.Getenv("AUTHENTIK_TOKEN"); t != "" {
		return strings.TrimSpace(t)
	}
	paths := []string{"/etc/keyagent/authentik_token", "/etc/keyagent/token", "/root/.authentik-token"}
	for _, p := range paths {
		if b, err := os.ReadFile(p); err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return s
			}
		}
	}
	// 尝试通过 secrets.ForFlavor() 从凭据面读取 dev/authentik/ci
	if p, err := secrets.ForFlavor(); err == nil {
		if m, err := p.Get(context.Background(), "dev/authentik/ci"); err == nil {
			if t := m["AUTHENTIK_TOKEN"]; t != "" {
				return strings.TrimSpace(t)
			}
		}
	}
	return ""
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
