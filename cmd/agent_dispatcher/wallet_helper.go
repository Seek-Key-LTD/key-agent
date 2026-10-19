// Copyright 2026 Google LLC
//
// Wallet and On-Chain Debit Card helper for KeyAgent Guest nodes
// Queries Base Sepolia balances and resolves EVM address for autonomous agent seats

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/adk/v2/internal/atoa"
)

var defaultBaseSepoliaRPCs = []string{
	"https://sepolia.base.org",
	"https://base-sepolia-rpc.publicnode.com",
}

// GetEVMAddress resolves the EVM debit card address of the seat
func GetEVMAddress(agentName, vaultAddr string) string {
	if addr := os.Getenv("EVM_ADDRESS"); addr != "" {
		return strings.TrimSpace(addr)
	}
	if data, err := os.ReadFile("/etc/keyagent/evm_address"); err == nil {
		if addr := strings.TrimSpace(string(data)); addr != "" {
			return addr
		}
	}
	if vaultAddr != "" && agentName != "" {
		_, evmAddr, _, _ := atoa.LoadASNCardFields(vaultAddr, agentName)
		if evmAddr != "" {
			return evmAddr
		}
	}
	return ""
}

type rpcRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  string          `json:"result"`
	Error   *rpcErrorDetail `json:"error,omitempty"`
}

type rpcErrorDetail struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// GetBaseSepoliaBalance queries the ETH balance on Base Sepolia testnet
func GetBaseSepoliaBalance(ctx context.Context, address string) (float64, error) {
	if address == "" {
		return 0, fmt.Errorf("empty EVM address")
	}

	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()

	payload := rpcRequest{
		JSONRPC: "2.0",
		Method:  "eth_getBalance",
		Params:  []interface{}{address, "latest"},
		ID:      1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return 0, err
	}

	client := &http.Client{Timeout: 4 * time.Second}

	var lastErr error
	for _, rpcURL := range defaultBaseSepoliaRPCs {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, rpcURL, bytes.NewReader(body))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		var res rpcResponse
		decodeErr := json.NewDecoder(resp.Body).Decode(&res)
		resp.Body.Close()

		if decodeErr != nil {
			lastErr = decodeErr
			continue
		}
		if res.Error != nil {
			lastErr = fmt.Errorf("RPC error %d: %s", res.Error.Code, res.Error.Message)
			continue
		}

		// Parse hex wei (e.g. 0x16345785d8a0000)
		hexStr := strings.TrimPrefix(res.Result, "0x")
		wei := new(big.Int)
		if _, ok := wei.SetString(hexStr, 16); !ok {
			lastErr = fmt.Errorf("failed to parse hex balance: %s", res.Result)
			continue
		}

		// Convert wei to ETH: float(wei) / 1e18
		fWei := new(big.Float).SetInt(wei)
		f1e18 := new(big.Float).SetFloat64(1e18)
		fEth := new(big.Float).Quo(fWei, f1e18)

		ethVal, _ := fEth.Float64()
		return ethVal, nil
	}

	if lastErr != nil {
		return 0, fmt.Errorf("all Base Sepolia RPC endpoints failed: %w", lastErr)
	}
	return 0, fmt.Errorf("failed to query balance")
}
