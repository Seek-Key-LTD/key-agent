package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// VaultProvider 读 KV v2 挂载：GET {addr}/v1/{mount}/data/{path}。
type VaultProvider struct {
	Addr   string
	Mount  string // 默认 "secret"
	Token  string
	Client *http.Client
}

func NewVault(addr string) (*VaultProvider, error) {
	if addr == "" {
		addr = "https://vault.capitaltrain.cn"
	}
	token := ReadVaultToken()
	if token == "" {
		return nil, fmt.Errorf("%w: vault token unavailable (checked /etc/vault-agent/token, /root/.vault-token, VAULT_TOKEN)", ErrNotConfigured)
	}
	return &VaultProvider{Addr: strings.TrimSuffix(addr, "/"), Mount: "secret", Token: token, Client: &http.Client{Timeout: 5 * time.Second}}, nil
}

func ReadVaultToken() string {
	for _, p := range []string{"/etc/vault-agent/token", "/root/.vault-token"} {
		if b, err := os.ReadFile(p); err == nil {
			return strings.TrimSpace(string(b))
		}
	}
	return os.Getenv("VAULT_TOKEN")
}

func (p *VaultProvider) Flavor() string { return "host" }

func (p *VaultProvider) Get(ctx context.Context, path string) (map[string]string, error) {
	url := fmt.Sprintf("%s/v1/%s/data/%s", p.Addr, p.Mount, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Vault-Token", p.Token)
	resp, err := p.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("vault get %s: status %d", path, resp.StatusCode)
	}
	var vr struct {
		Data struct {
			Data map[string]any `json:"data"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, err
	}
	return stringify(vr.Data.Data), nil
}

func stringify(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			out[k] = s
			continue
		}
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}
