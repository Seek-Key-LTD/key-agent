//go:build guest

package secrets

import "fmt"

const defaultFlavor = FlavorGuest

func newPlatformProvider() (Provider, error) {
	return nil, fmt.Errorf("%w: guest 产物未编译入平台凭据 Provider，flavor 应为 %s", ErrNotConfigured, FlavorGuest)
}
