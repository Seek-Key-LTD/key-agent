// Package secrets 提供 key-agent 的双 flavor 凭据面。产物按 Go build tag 物理分包：
//   host  —— 默认 `go build`：包含平台凭据 Provider（platform_host.go / vault.go）
//   guest —— `go build -tags guest`：二进制内不含平台 Provider 的任何代码与字符串，
//   仅走 Infisical 纯出站 HTTPS（machine identity universal auth）。
//
// flavor 选择：进程 env KEYAGENT_FLAVOR 优先，其次 /etc/keyagent/config.json 的
// flavor 字段（launcher 注入），都未设置时取构建默认（host 构建=host，guest 构建=guest）。
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

// 端点默认值（约定真相源：repo wiki「Key-Agent-Predefines」§2）。
const (
	DefaultInfisicalURL = "https://infisical.capitaltrain.cn"

	FlavorHost  = "host"
	FlavorGuest = "guest"
)

// ForFlavor 按当前 flavor 构造 Provider。
func ForFlavor() (Provider, error) {
	switch Flavor() {
	case FlavorGuest:
		return NewInfisicalFromEnv()
	default:
		return newPlatformProvider()
	}
}

// Flavor 返回生效 flavor。
func Flavor() string {
	f := os.Getenv("KEYAGENT_FLAVOR")
	if f == "" {
		return defaultFlavor
	}
	return f
}

func Get(path string) (map[string]string, error) {
	p, err := ForFlavor()
	if err != nil {
		return nil, err
	}
	return p.Get(context.Background(), path)
}

var ErrNotConfigured = fmt.Errorf("secret provider not configured")
