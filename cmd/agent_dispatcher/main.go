// Copyright 2026 Google LLC
//
// Agent Dispatcher Daemon Service
// Listens for Gitea Webhooks, logs to Memory Bank, locks Feishu Calendar Occupations,
// and manages Unified Communications across Matrix and Mastodon channels.

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/adk/v2/integration"
	"google.golang.org/adk/v2/internal/atoa"
	"google.golang.org/adk/v2/messaging"
)

// PicoConfigFile represents local node configuration (e.g. from /root/.picooraclaw/config.json)
type PicoConfigFile struct {
	Channels struct {
		Matrix struct {
			Enabled     bool     `json:"enabled"`
			Homeserver  string   `json:"homeserver"`
			UserID      string   `json:"user_id"`
			AccessToken string   `json:"access_token"`
			AllowFrom   []string `json:"allow_from"`
		} `json:"matrix"`
		Mastodon struct {
			Enabled     bool   `json:"enabled"`
			Server      string `json:"server"`
			AccessToken string `json:"access_token"`
		} `json:"mastodon"`
	} `json:"channels"`
	Oracle struct {
		AgentID string `json:"agent_id"`
	} `json:"oracle"`
}

func loadPicoConfig(path string) *PicoConfigFile {
	if path == "" {
		if envPath := os.Getenv("PICO_CONFIG"); envPath != "" {
			path = envPath
		} else if _, err := os.Stat("/root/.picooraclaw/config.json"); err == nil {
			path = "/root/.picooraclaw/config.json"
		} else if home, err := os.UserHomeDir(); err == nil {
			candidate := home + "/.picooraclaw/config.json"
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			}
		}
	}
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("[Config] ⚠️ Could not read config from %s: %v", path, err)
		return nil
	}
	var cfg PicoConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[Config] ⚠️ Could not parse config JSON from %s: %v", path, err)
		return nil
	}
	log.Printf("[Config] ✅ Loaded configuration from %s", path)
	return &cfg
}

func main() {
	// Parse command-line arguments (support: --config <path>, --config=<path>, gateway, version)
	var configPath string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "version" || arg == "-v" || arg == "--version" {
			fmt.Println("KeyAgent v2.4.0-matrix-mastodon (ADK-Go core)")
			return
		}
		if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			i++
		} else if strings.HasPrefix(arg, "--config=") {
			configPath = strings.TrimPrefix(arg, "--config=")
		}
	}

	picoCfg := loadPicoConfig(configPath)

	port := os.Getenv("PORT")
	if port == "" {
		port = "18790"
	}

	authentikURL := os.Getenv("AUTHENTIK_URL")
	if authentikURL == "" {
		authentikURL = "https://authentik.capitaltrain.cn"
	}

	vaultAddr := os.Getenv("VAULT_ADDR")
	if vaultAddr == "" {
		vaultAddr = "https://vault.capitaltrain.cn"
	}

	// Resolve agent identity
	agentName := os.Getenv("AGENT_NAME")
	if agentName == "" && picoCfg != nil {
		if picoCfg.Oracle.AgentID != "" {
			agentName = picoCfg.Oracle.AgentID
		} else if picoCfg.Channels.Matrix.UserID != "" {
			parts := strings.Split(picoCfg.Channels.Matrix.UserID, ":")
			agentName = strings.TrimPrefix(parts[0], "@")
		}
	}
	if agentName == "" {
		agentName = os.Getenv("NODE_NAME")
	}
	if agentName == "" {
		if h, err := os.Hostname(); err == nil {
			agentName = h
		} else {
			agentName = "agent"
		}
	}

	dispatcher := integration.NewDispatcherService(authentikURL, vaultAddr)

	// 读取 ASN 自定义名片字段（soul_name/evm_address/constellation/coherence）
	soulName, evmAddr, constellation, coherence := atoa.LoadASNCardFields(vaultAddr, agentName)

	// 1. Matrix Channel Initialization
	matrixServer := os.Getenv("MATRIX_HOMESERVER")
	matrixToken := os.Getenv("MATRIX_TOKEN")
	matrixUserID := os.Getenv("MATRIX_USER_ID")

	if picoCfg != nil && picoCfg.Channels.Matrix.Enabled {
		if matrixServer == "" {
			matrixServer = picoCfg.Channels.Matrix.Homeserver
		}
		if matrixToken == "" {
			matrixToken = picoCfg.Channels.Matrix.AccessToken
		}
		if matrixUserID == "" {
			matrixUserID = picoCfg.Channels.Matrix.UserID
		}
	}
	if matrixServer == "" {
		matrixServer = "https://matrix.git4ta.fun"
	}
	if matrixUserID == "" && agentName != "" {
		matrixUserID = fmt.Sprintf("@%s:matrix.git4ta.fun", agentName)
	}

	var matrixClient *messaging.MatrixClient
	if matrixToken != "" {
		matrixClient = messaging.NewMatrixClient(messaging.MatrixConfig{
			HomeserverURL: matrixServer,
			AccessToken:   matrixToken,
			UserID:        matrixUserID,
		})
		ctx := context.Background()
		// Start Matrix presence keepalive (every 45s)
		go matrixClient.StartPresenceLoop(ctx, 45*time.Second)

		// Start Matrix long-polling sync loop
		go matrixClient.StartSync(ctx, func(roomID, sender, text, eventID string) {
			log.Printf("[Matrix Comm] 📥 [%s] %s: %s (Event: %s)", roomID, sender, text, eventID)
			go func() {
				_, _ = matrixClient.SendReaction(context.Background(), roomID, eventID, "👀")
			}()
		})
		log.Printf("[Matrix Comm] 🟢 Channel active for %s on %s", matrixUserID, matrixServer)
	} else {
		log.Printf("[Matrix Comm] ⚪ Matrix channel not configured for %s", agentName)
	}

	// 2. Mastodon Channel Initialization
	mastodonServer := os.Getenv("MASTODON_SERVER")
	if mastodonServer == "" {
		if picoCfg != nil && picoCfg.Channels.Mastodon.Server != "" {
			mastodonServer = picoCfg.Channels.Mastodon.Server
		} else {
			mastodonServer = "https://mastodon.capitaltrain.cn"
		}
	}
	mastodonToken := os.Getenv("MASTODON_TOKEN")
	if mastodonToken == "" && picoCfg != nil && picoCfg.Channels.Mastodon.AccessToken != "" {
		mastodonToken = picoCfg.Channels.Mastodon.AccessToken
	}
	if mastodonToken == "" && agentName != "" {
		// Attempt Consul KV lookup: mastodon/clients/<agentName>/token
		consulURL := fmt.Sprintf("http://127.0.0.1:8500/v1/kv/mastodon/clients/%s/token?raw", agentName)
		req, _ := http.NewRequest(http.MethodGet, consulURL, nil)
		client := &http.Client{Timeout: 2 * time.Second}
		if resp, err := client.Do(req); err == nil && resp.StatusCode == http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			mastodonToken = strings.TrimSpace(string(b))
		}
	}

	var mastodonClient *messaging.MastodonClient
	if mastodonToken != "" {
		mastodonClient = messaging.NewMastodonClient(messaging.MastodonConfig{
			ServerURL:   mastodonServer,
			AccessToken: mastodonToken,
			AgentName:   agentName,
		})
		ctx := context.Background()
		acc, err := mastodonClient.VerifyCredentials(ctx)
		if err != nil {
			log.Printf("[Mastodon Comm] ⚠️ Failed to verify Mastodon credentials: %v", err)
		} else {
			log.Printf("[Mastodon Comm] 🟢 Logged in as @%s on %s (ID: %s)", acc.Acct, mastodonServer, acc.ID)
			go mastodonClient.StartNotificationLoop(ctx, 30*time.Second, func(n *messaging.MastodonNotification) {
				sender := "unknown"
				if n.Account != nil {
					sender = n.Account.Acct
				}
				content := ""
				if n.Status != nil {
					content = n.Status.Content
				}
				log.Printf("[Mastodon Comm] 🔔 Mention from @%s: %s (Notification ID: %s)", sender, content, n.ID)
			})
		}
	} else {
		log.Printf("[Mastodon Comm] ⚪ Mastodon channel not configured for %s", agentName)
	}

	// 3. HTTP Server Routing & A2A
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]interface{}{
			"status":     "healthy",
			"agent_name": agentName,
			"soul_name":  soulName,
			"matrix": map[string]interface{}{
				"enabled": matrixClient != nil,
				"user_id": matrixUserID,
				"server":  matrixServer,
			},
			"mastodon": map[string]interface{}{
				"enabled": mastodonClient != nil,
				"server":  mastodonServer,
			},
			"time": time.Now().Format(time.RFC3339),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	atoa.Mount(mux,
		agentName,
		os.Getenv("AGENT_DESCRIPTION"),
		os.Getenv("AGENT_ADDR"),
		"https://authentik.capitaltrain.cn/application/o/userinfo/",
		atoa.LLMConfig{
			BaseURL: os.Getenv("LLM_BASE_URL"),
			APIKey:  os.Getenv("LLM_API_KEY"),
			Model:   os.Getenv("LLM_MODEL"),
		},
		soulName, evmAddr, constellation, coherence,
	)

	// A2A 客户端：主动调用别的 agent (POST /atoa/call {"to":"topaz","task":"..."})
	mux.HandleFunc("/atoa/call", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			To   string `json:"to"`
			Task string `json:"task"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.To == "" || req.Task == "" {
			http.Error(w, `{"error":"to and task required"}`, http.StatusBadRequest)
			return
		}
		client, err := atoa.NewClientFromEnv()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		reply, err := client.SendMessage(r.Context(), req.To, req.Task)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"from": client.From, "to": req.To, "reply": reply})
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
	log.Printf("🚀 Starting KeyAgent Unified Daemon for [%s] on :%s", agentName, port)
	log.Printf("   - Gitea Webhook URL: http://localhost:%s/webhook/gitea", port)
	log.Printf("   - Matrix User:       %s (%s)", matrixUserID, matrixServer)
	log.Printf("   - Mastodon Server:   %s", mastodonServer)
	log.Printf("   - Authentik SSO:     %s", authentikURL)
	log.Printf("   - Vault Secret Path: %s", vaultAddr)
	log.Println("==================================================================")

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Daemon stopped unexpectedly: %v", err)
	}
}
