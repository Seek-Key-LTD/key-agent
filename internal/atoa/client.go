// A2A 客户端：Consul delegations 发现 + authentik 认证 + 调用远程 agent。
package atoa

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"os"
	"time"

	"github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"

	"google.golang.org/adk/v2/auth"
)

// Registry 是 A2A agent 发现接口（用 Consul KV delegations 实现）。
type Registry interface {
	// Endpoint 返回 agent 的 A2A 端点地址。
	Endpoint(ctx context.Context, agentName string) (string, error)
}

// ConsulRegistry 从 Consul KV delegations/agents/<name> 读 endpoint。
type ConsulRegistry struct {
	ConsulAddr string // 如 http://127.0.0.1:8500
	HTTPClient *http.Client
}

// Endpoint 实现 Registry：读 Consul KV delegations/agents/<name> 的 endpoint 字段。
func (r *ConsulRegistry) Endpoint(ctx context.Context, agentName string) (string, error) {
	client := r.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}

	// 1. 读 delegations/agents/<name> 拿 node 名
	kvURL := fmt.Sprintf("%s/v1/kv/delegations/agents/%s?raw", r.ConsulAddr, agentName)
	req, err := http.NewRequestWithContext(ctx, "GET", kvURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("consul: query %s: %w", agentName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("consul: agent %s not found (status %d)", agentName, resp.StatusCode)
	}
	var entry struct {
		Node string `json:"node"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return "", fmt.Errorf("consul: decode %s: %w", agentName, err)
	}
	if entry.Node == "" {
		return "", fmt.Errorf("consul: agent %s has no node", agentName)
	}

	// 2. catalog API 跨 DC 拿节点 IP（dc1=Area 0 权威，遍历 dc1/dc2/dc3）
	//    不依赖 Consul DNS；节点可能分布在任意 DC，逐个 DC 查 catalog。
	dcs := []string{"dc1", "dc2", "dc3"}
	var nodeAddr string
	for _, dc := range dcs {
		catURL := fmt.Sprintf("%s/v1/catalog/node/%s?dc=%s", r.ConsulAddr, entry.Node, dc)
		catReq, err := http.NewRequestWithContext(ctx, "GET", catURL, nil)
		if err != nil {
			continue
		}
		catResp, err := client.Do(catReq)
		if err != nil {
			continue
		}
		var cat struct {
			Node struct {
				Address string `json:"Address"`
			} `json:"Node"`
		}
		decErr := json.NewDecoder(catResp.Body).Decode(&cat)
		catResp.Body.Close()
		if decErr != nil || cat.Node.Address == "" {
			continue
		}
		// A2A 底线: 必须走 Tailscale (100.x), 私网/公网地址一律拒绝
		if !strings.HasPrefix(cat.Node.Address, "100.") {
			continue
		}
		nodeAddr = cat.Node.Address
		break
	}
	if nodeAddr == "" {
		return "", fmt.Errorf("consul: node %s not reachable via Tailscale in any DC (%v)", entry.Node, dcs)
	}

	return fmt.Sprintf("http://%s:18790/a2a", nodeAddr), nil
}

// Client 封装 A2A 远程调用。
type Client struct {
	// From 是调用方 agent 名（用于日志/追踪）。
	From string
	// Registry 负责 agent 发现。
	Registry Registry
	// AuthProvider 提供调用方 authentik 凭据（nil 则无认证）。
	AuthProvider auth.CredentialProvider
	// HTTPClient 底层 HTTP 客户端（带 auth transport）。
	HTTPClient *http.Client
}

// SendMessage 调远程 agent 的 A2A SendMessage。
func (c *Client) SendMessage(ctx context.Context, toAgent, taskText string) (string, error) {
	endpoint, err := c.Registry.Endpoint(ctx, toAgent)
	if err != nil {
		return "", err
	}

	// 底层 client：挂 auth.Transport 注入 authentik Bearer token
	httpClient := c.HTTPClient
	if httpClient == nil {
		base := http.DefaultTransport
		httpClient = &http.Client{
			Transport: base,
			Timeout:   90 * time.Second,
		}
	}
	if c.AuthProvider != nil {
		httpClient.Transport = &auth.Transport{Provider: c.AuthProvider, Base: httpClient.Transport}
	}

	a2aClient, err := a2aclient.NewFromEndpoints(
		ctx,
		[]*a2a.AgentInterface{{URL: endpoint, ProtocolBinding: a2a.TransportProtocolJSONRPC, ProtocolVersion: a2a.Version}},
		a2aclient.WithConfig(a2aclient.Config{
			PreferredTransports: []a2a.TransportProtocol{a2a.TransportProtocolJSONRPC},
		}),
		a2aclient.WithJSONRPCTransport(httpClient),
	)
	if err != nil {
		return "", fmt.Errorf("a2a: create client for %s: %w", toAgent, err)
	}

	msg := &a2a.Message{
		ID: fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Role:      a2a.MessageRoleUser,
		Parts:     []*a2a.Part{a2a.NewTextPart(taskText)},
	}
	res, err := a2aClient.SendMessage(ctx, &a2a.SendMessageRequest{Message: msg})
	if err != nil {
		return "", fmt.Errorf("a2a: SendMessage to %s: %w", toAgent, err)
	}

	// 提取回复文本 (result 是 *Message 或 *Task)
	switch r := res.(type) {
	case *a2a.Message:
		var sb string
		for _, p := range r.Parts {
			sb += p.Text()
		}
		return sb, nil
	case *a2a.Task:
		for _, m := range r.History {
			if m.Role == a2a.MessageRoleAgent {
				var sb string
				for _, p := range m.Parts {
					sb += p.Text()
				}
				if sb != "" {
					return sb, nil
				}
			}
		}
	}
	return "", fmt.Errorf("a2a: no agent reply from %s", toAgent)
}

// NewClientFromEnv 从环境变量构造 Client：
// AUTHENTIK_URL / AUTHENTIK_CLIENT_ID / AUTHENTIK_CLIENT_SECRET (调用方凭据)
// CONSUL_ADDR (registry, 默认 http://127.0.0.1:8500)
func NewClientFromEnv() (*Client, error) {
	from := os.Getenv("AGENT_NAME")
	if from == "" {
		from = "unknown"
	}
	consulAddr := os.Getenv("CONSUL_ADDR")
	if consulAddr == "" {
		consulAddr = "http://127.0.0.1:8500"
	}

	client := &Client{
		From:     from,
		Registry: &ConsulRegistry{ConsulAddr: consulAddr},
	}

	// 调用方凭据（可选）：有 authentik env 就挂认证
	if os.Getenv("AUTHENTIK_CLIENT_ID") != "" {
		provider, err := auth.AuthentikFromEnv()
		if err != nil {
			return nil, fmt.Errorf("auth: %w", err)
		}
		client.AuthProvider = provider
	}
	return client, nil
}
