// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package auth_test

import (
	"context"
	"testing"

	"google.golang.org/adk/v2/auth"
)

func TestAuthentikClientCredentials_MissingConfig(t *testing.T) {
	ctx := context.Background()

	provider := auth.AuthentikClientCredentials(auth.AuthentikConfig{})
	_, err := provider.Credential(ctx)
	if err == nil {
		t.Errorf("expected error for empty config, got nil")
	}
}

func TestAuthentikFromEnv(t *testing.T) {
	t.Setenv("AUTHENTIK_URL", "https://authentik.capitaltrain.cn")
	t.Setenv("AUTHENTIK_CLIENT_ID", "agent-client-amber")
	t.Setenv("AUTHENTIK_CLIENT_SECRET", "secret-key-123")

	provider, err := auth.AuthentikFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider == nil {
		t.Fatalf("expected non-nil provider")
	}
}
