package secrets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVaultProviderGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Vault-Token"); got != "tok" {
			t.Errorf("token = %q", got)
		}
		if r.URL.Path != "/v1/secret/data/dev/agentcard/amber" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"data": map[string]any{"soul_name": "琥珀", "evm_address": "0x1", "constellation_house": "liao"}},
		})
	}))
	defer srv.Close()
	t.Setenv("VAULT_TOKEN", "tok")
	p, err := NewVault(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	m, err := p.Get(context.Background(), "dev/agentcard/amber")
	if err != nil {
		t.Fatal(err)
	}
	if m["soul_name"] != "琥珀" || m["evm_address"] != "0x1" {
		t.Fatalf("got %v", m)
	}
}

func TestInfisicalProviderGet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/universal-auth/login":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			if in["clientId"] != "cid" || in["clientSecret"] != "csec" {
				t.Errorf("creds = %v", in)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"accessToken": "at", "expireIn": 7200})
		case "/api/v3/secrets/raw":
			if r.Header.Get("Authorization") != "Bearer at" {
				t.Errorf("auth header = %q", r.Header.Get("Authorization"))
			}
			if got := r.URL.Query().Get("secretPath"); got != "/dev/agentcard/amber" {
				t.Errorf("secretPath = %q", got)
			}
			// 实机 v0.165.16 只接受 workspaceId 参数（projectId 会被 400 拒绝）
			if got := r.URL.Query().Get("workspaceId"); got != "pid" {
				t.Errorf("workspaceId = %q", got)
			}
			// 实机响应为顶层 secrets 数组
			_ = json.NewEncoder(w).Encode(map[string]any{
				"secrets": []any{
					map[string]any{"secretKey": "soul_name", "secretValue": "琥珀"},
					map[string]any{"secretKey": "evm_address", "secretValue": "0x2"},
				},
			})
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()
	t.Setenv("INFISICAL_URL", srv.URL)
	t.Setenv("INFISICAL_CLIENT_ID", "cid")
	t.Setenv("INFISICAL_CLIENT_SECRET", "csec")
	t.Setenv("INFISICAL_PROJECT_ID", "pid")
	t.Setenv("INFISICAL_ENV", "dev")
	p, err := NewInfisicalFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	m, err := p.Get(context.Background(), "dev/agentcard/amber")
	if err != nil {
		t.Fatal(err)
	}
	if m["soul_name"] != "琥珀" || m["evm_address"] != "0x2" {
		t.Fatalf("got %v", m)
	}
	if _, err := p.Get(context.Background(), "../etc/passwd"); err == nil {
		t.Fatal("path traversal should be rejected")
	}
}

func TestForFlavorDefault(t *testing.T) {
	t.Setenv("KEYAGENT_FLAVOR", "")
	t.Setenv("VAULT_TOKEN", "tok")
	p, err := ForFlavor("http://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	if p.Flavor() != "host" {
		t.Fatalf("flavor = %q", p.Flavor())
	}
}
