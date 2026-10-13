//go:build !guest

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

func TestForFlavorDefaultHost(t *testing.T) {
	t.Setenv("KEYAGENT_FLAVOR", "")
	t.Setenv("VAULT_ADDR", "http://127.0.0.1:1")
	t.Setenv("VAULT_TOKEN", "tok")
	p, err := ForFlavor()
	if err != nil {
		t.Fatal(err)
	}
	if p.Flavor() != "host" {
		t.Fatalf("flavor = %q", p.Flavor())
	}
}
