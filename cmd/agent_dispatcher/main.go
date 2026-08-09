// Copyright 2026 Google LLC
//
// Agent Dispatcher Daemon Service
// Listens for Gitea Webhooks, logs to Memory Bank, and locks Feishu Calendar Occupations

package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"google.golang.org/adk/v2/integration"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8090"
	}

	authentikURL := os.Getenv("AUTHENTIK_URL")
	if authentikURL == "" {
		authentikURL = "https://authentik.capitaltrain.cn"
	}

	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		vaultAddr = "http://192.168.31.111:8200"
	}

	dispatcher := integration.NewDispatcherService(authentikURL, vaultAddr)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/webhook/gitea", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx := r.Context()
		res, err := dispatcher.HandleGiteaWebhook(ctx, r)
		if err != nil {
			log.Printf("[Daemon] Webhook handling error: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(res)
	})

	log.Println("==================================================================")
	log.Printf("🚀 Starting Agent Dispatcher Daemon on :%s", port)
	log.Printf("   - Gitea Webhook URL: http://localhost:%s/webhook/gitea", port)
	log.Printf("   - Authentik SSO:     %s", authentikURL)
	log.Printf("   - Vault Secret Path: %s", vaultAddr)
	log.Println("==================================================================")

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Daemon stopped unexpectedly: %v", err)
	}
}
