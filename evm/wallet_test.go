// Copyright 2026 Google LLC
//
// EVM Tipping Unit Tests

package evm

import (
	"context"
	"testing"
)

func TestEVMWalletAndTipping(t *testing.T) {
	topazWallet, err := GenerateAgentWallet("topaz")
	if err != nil || topazWallet.Address == "" {
		t.Fatalf("Failed to generate Topaz wallet: %v", err)
	}

	rubyWallet, err := GenerateAgentWallet("ruby")
	if err != nil || rubyWallet.Address == "" {
		t.Fatalf("Failed to generate Ruby wallet: %v", err)
	}

	ctx := context.Background()
	tip, err := SendAgentTip(ctx, topazWallet, "ruby", rubyWallet.Address, "0.05", "Reward for solving 1+1 Python task")
	if err != nil {
		t.Fatalf("SendAgentTip failed: %v", err)
	}

	if tip.Status != "CONFIRMED (Block #1948201)" {
		t.Errorf("Unexpected tip status: %s", tip.Status)
	}
}
