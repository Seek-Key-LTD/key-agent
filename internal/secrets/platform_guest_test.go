//go:build guest

package secrets

import "testing"

func TestForFlavorDefaultGuest(t *testing.T) {
	t.Setenv("KEYAGENT_FLAVOR", "")
	t.Setenv("INFISICAL_URL", "https://infisical.invalid")
	t.Setenv("INFISICAL_CLIENT_ID", "cid")
	t.Setenv("INFISICAL_CLIENT_SECRET", "csec")
	t.Setenv("INFISICAL_PROJECT_ID", "pid")
	p, err := ForFlavor()
	if err != nil {
		t.Fatal(err)
	}
	if p.Flavor() != "guest" {
		t.Fatalf("flavor = %q", p.Flavor())
	}
}

func TestGuestRejectsHostFlavor(t *testing.T) {
	t.Setenv("KEYAGENT_FLAVOR", "host")
	if _, err := ForFlavor(); err == nil {
		t.Fatal("guest 产物必须拒绝 host flavor")
	}
}
