// Package secrets 提供 key-agent 的双 flavor 凭据面：
//   host  —— 内网完整形态：Vault（含 vault-agent 本地 token）
//   guest —— 外围形态：Infisical over 纯出站 HTTPS（machine identity universal auth）
//
// 选路：环境变量 KEYAGENT_FLAVOR=host|guest（默认 host）。
package secrets

import (
	"context"
	"fmt"
	"os"
)

// Provider 读取一个逻辑路径下的 KV（如 dev/agentcard/amber）。
type Provider interface {
	Flavor() string
	Get(ctx context.Context, path string) (map[string]string, error)
}

// ForFlavor 按当前 flavor 构造 Provider。
func ForFlavor(vaultAddr string) (Provider, error) {
	switch flavor() {
	case "guest":
		return NewInfisicalFromEnv()
	default:
		return NewVault(vaultAddr)
	}
}

func flavor() string {
	f := os.Getenv("KEYAGENT_FLAVOR")
	if f == "" {
		return "host"
	}
	return f
}

func Get(path string) (map[string]string, error) {
	p, err := ForFlavor(os.Getenv("VAULT_ADDR"))
	if err != nil {
		return nil, err
	}
	return p.Get(context.Background(), path)
}

var ErrNotConfigured = fmt.Errorf("secret provider not configured")
