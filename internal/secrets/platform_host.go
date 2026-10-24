//go:build !guest

package secrets

import "os"

const defaultFlavor = FlavorHost

// DefaultVaultAddr 内网 Vault 默认端点。
const DefaultVaultAddr = "https://vault.capitaltrain.cn"

func newPlatformProvider() (Provider, error) {
	return NewVault(os.Getenv("VAULT_ADDR"))
}
