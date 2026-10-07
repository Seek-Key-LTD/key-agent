package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// InfisicalProvider 走 machine identity universal auth，纯出站 HTTPS。
// 逻辑路径 "dev/agentcard/amber" 映射为 secretPath "/dev/agentcard/amber"。
type InfisicalProvider struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	ProjectID    string
	Environment  string
	Client       *http.Client

	mu       sync.Mutex
	token    string
	expireAt time.Time
}

func NewInfisicalFromEnv() (*InfisicalProvider, error) {
	base := os.Getenv("INFISICAL_URL")
	if base == "" {
		base = DefaultInfisicalURL
	}
	p := &InfisicalProvider{
		BaseURL:      strings.TrimSuffix(base, "/"),
		ClientID:     os.Getenv("INFISICAL_CLIENT_ID"),
		ClientSecret: os.Getenv("INFISICAL_CLIENT_SECRET"),
		ProjectID:    os.Getenv("INFISICAL_PROJECT_ID"),
		Environment:  os.Getenv("INFISICAL_ENV"),
		Client:       &http.Client{Timeout: 10 * time.Second},
	}
	if p.Environment == "" {
		p.Environment = "prod"
	}
	if p.ClientID == "" || p.ClientSecret == "" || p.ProjectID == "" {
		return nil, fmt.Errorf("%w: need INFISICAL_CLIENT_ID/INFISICAL_CLIENT_SECRET/INFISICAL_PROJECT_ID", ErrNotConfigured)
	}
	return p, nil
}

func (p *InfisicalProvider) Flavor() string { return "guest" }

func (p *InfisicalProvider) login(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.token != "" && time.Until(p.expireAt) > time.Minute {
		return p.token, nil
	}
	body, _ := json.Marshal(map[string]string{
		"clientId":     p.ClientID,
		"clientSecret": p.ClientSecret,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/api/v1/auth/universal-auth/login", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("infisical login: status %d: %s", resp.StatusCode, b)
	}
	var lr struct {
		AccessToken    string `json:"accessToken"`
		AuthProxyToken string `json:"authProxyToken"`
		ExpireIn       int    `json:"expireIn"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return "", err
	}
	tok := lr.AccessToken
	if tok == "" {
		tok = lr.AuthProxyToken
	}
	if tok == "" {
		return "", fmt.Errorf("infisical login: empty access token")
	}
	ttl := time.Duration(lr.ExpireIn) * time.Second
	if lr.ExpireIn == 0 {
		ttl = time.Hour
	}
	p.token = tok
	p.expireAt = time.Now().Add(ttl)
	return tok, nil
}

func (p *InfisicalProvider) Get(ctx context.Context, path string) (map[string]string, error) {
	if err := validateRelPath(path); err != nil {
		return nil, err
	}
	tok, err := p.login(ctx)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("workspaceId", p.ProjectID)
	q.Set("environment", p.Environment)
	q.Set("secretPath", "/"+path)
	u := p.BaseURL + "/api/v3/secrets/raw?" + q.Encode()

	get := func(t string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+t)
		return p.Client.Do(req)
	}
	resp, err := get(tok)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()
		p.mu.Lock()
		p.token = ""
		p.mu.Unlock()
		if tok, err = p.login(ctx); err != nil {
			return nil, err
		}
		if resp, err = get(tok); err != nil {
			return nil, err
		}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("infisical get %s: status %d: %s", path, resp.StatusCode, b)
	}
	var raw map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, err
	}
	secretsList := dig(raw, "secrets")
	if secretsList == nil {
		secretsList = dig(raw, "data", "result", "secrets")
	}
	if secretsList == nil {
		secretsList = dig(raw, "result", "secrets")
	}
	if secretsList == nil {
		secretsList = dig(raw, "data", "secrets")
	}
	items, _ := secretsList.([]any)
	if items == nil {
		return nil, fmt.Errorf("infisical get %s: no secrets array in response", path)
	}
	out := make(map[string]string, len(items))
	for _, it := range items {
		m, _ := it.(map[string]any)
		if m == nil {
			continue
		}
		k, _ := m["secretKey"].(string)
		if k == "" {
			continue
		}
		if vs, ok := m["secretValue"].(string); ok {
			out[k] = vs
		} else {
			out[k] = fmt.Sprintf("%v", m["secretValue"])
		}
	}
	return out, nil
}

func dig(m map[string]any, keys ...string) any {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = mm[k]
		if !ok {
			return nil
		}
	}
	return cur
}

func validateRelPath(path string) error {
	if path == "" || strings.Contains(path, "..") || strings.HasPrefix(path, "/") {
		return fmt.Errorf("invalid secret path %q", path)
	}
	return nil
}
