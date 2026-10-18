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

	"google.golang.org/adk/v2/feishu"
	"google.golang.org/adk/v2/integration"
	"google.golang.org/adk/v2/internal/atoa"
	"google.golang.org/adk/v2/messaging"
)

// KeyAgentConfigFile represents local node configuration (e.g. from /etc/keyagent/config.json)
type KeyAgentConfigFile struct {
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

func loadKeyAgentConfig(path string) *KeyAgentConfigFile {
	if path == "" {
		if envPath := os.Getenv("KEYAGENT_CONFIG"); envPath != "" {
			path = envPath
		} else if envPath := os.Getenv("PICO_CONFIG"); envPath != "" {
			path = envPath
		} else if _, err := os.Stat("/etc/keyagent/config.json"); err == nil {
			path = "/etc/keyagent/config.json"
		} else if home, err := os.UserHomeDir(); err == nil && fileExists(home+"/.keyagent/config.json") {
			path = home + "/.keyagent/config.json"
		} else if home, err := os.UserHomeDir(); err == nil && fileExists(home+"/.config/keyagent/config.json") {
			path = home + "/.config/keyagent/config.json"
		} else if _, err := os.Stat("/root/.picooraclaw/config.json"); err == nil {
			log.Printf("[Config] ⚠️ Legacy config path /root/.picooraclaw/config.json detected, please migrate to /etc/keyagent/config.json")
			path = "/root/.picooraclaw/config.json"
		} else if home, err := os.UserHomeDir(); err == nil && fileExists(home+"/.picooraclaw/config.json") {
			log.Printf("[Config] ⚠️ Legacy config path %s/.picooraclaw/config.json detected, please migrate to %s/.keyagent/config.json", home, home)
			path = home + "/.picooraclaw/config.json"
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
	var cfg KeyAgentConfigFile
	if err := json.Unmarshal(data, &cfg); err != nil {
		log.Printf("[Config] ⚠️ Could not parse config JSON from %s: %v", path, err)
		return nil
	}
	log.Printf("[Config] ✅ Loaded configuration from %s", path)
	return &cfg
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

var Version = "v2.5.1-host"

func main() {
	// 0. Handle native Feishu subcommands (e.g. k-agent feishu im-send ...)
	if len(os.Args) > 1 && os.Args[1] == "feishu" {
		if handleFeishuCommand(os.Args[2:]) {
			return
		}
	}

	// Parse command-line arguments (support: --config <path>, --config=<path>, gateway, version)
	var configPath string
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "version" || arg == "-v" || arg == "--version" {
			if strings.Contains(Version, "guest") {
				fmt.Printf("KeyAgent %s (ADK-Go core - Decoupled Guest Edition)\n", Version)
			} else {
				fmt.Printf("KeyAgent %s (ADK-Go core - Host Edition with Vault & HAProxy Memory Hub)\n", Version)
			}
			return
		}
		if arg == "--config" && i+1 < len(os.Args) {
			configPath = os.Args[i+1]
			i++
		} else if strings.HasPrefix(arg, "--config=") {
			configPath = strings.TrimPrefix(arg, "--config=")
		}
	}

	agentCfg := loadKeyAgentConfig(configPath)

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

	// Resolve agent identity (stone name from config takes priority over generic hostname/NODE_NAME)
	agentName := ""
	if agentCfg != nil {
		if agentCfg.Oracle.AgentID != "" {
			agentName = agentCfg.Oracle.AgentID
		} else if agentCfg.Channels.Matrix.UserID != "" {
			parts := strings.Split(agentCfg.Channels.Matrix.UserID, ":")
			agentName = strings.TrimPrefix(parts[0], "@")
		}
	}
	if agentName == "" {
		agentName = os.Getenv("AGENT_NAME")
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

	if agentCfg != nil && agentCfg.Channels.Matrix.Enabled {
		if matrixServer == "" {
			matrixServer = agentCfg.Channels.Matrix.Homeserver
		}
		if matrixToken == "" {
			matrixToken = agentCfg.Channels.Matrix.AccessToken
		}
		if matrixUserID == "" {
			matrixUserID = agentCfg.Channels.Matrix.UserID
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
		if agentCfg != nil && agentCfg.Channels.Mastodon.Server != "" {
			mastodonServer = agentCfg.Channels.Mastodon.Server
		} else {
			mastodonServer = "https://mastodon.capitaltrain.cn"
		}
	}
	mastodonToken := os.Getenv("MASTODON_TOKEN")
	if mastodonToken == "" && agentCfg != nil && agentCfg.Channels.Mastodon.AccessToken != "" {
		mastodonToken = agentCfg.Channels.Mastodon.AccessToken
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

				// Auto-respond to Mention with host metrics (CPU / Memory)
				if n.Status != nil && n.Account != nil && !n.Account.Bot {
					replyTarget := n.Account.Acct
					metrics := getSystemMetrics()
					replyContent := fmt.Sprintf("@%s 报告！[%s 席位] 已捕获点名调度。\n📊 主机实时指标：\n• CPU 负载(1/5/15m): %s\n• 内存状态: %s / %s (已用 %s)",
						replyTarget, agentName, metrics.LoadAvg, metrics.MemUsed, metrics.MemTotal, metrics.MemPercent)

					go func(statusID, replyText string) {
						time.Sleep(1 * time.Second)
						if st, err := mastodonClient.PostStatus(context.Background(), replyText, statusID, "public"); err != nil {
							log.Printf("[Mastodon Comm] ⚠️ Failed to auto-reply to mention %s: %v", statusID, err)
						} else {
							log.Printf("[Mastodon Comm] 📤 Auto-replied to @%s (Reply ID: %s, URL: %s)", replyTarget, st.ID, st.URL)
						}
					}(n.Status.ID, replyContent)
				}
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

	feishuWebhookHandler := feishu.Webhook(feishu.WebhookConfig{
		VerificationToken: os.Getenv("FEISHU_VERIFICATION_TOKEN"),
		EncryptKey:        os.Getenv("FEISHU_ENCRYPT_KEY"),
	}, func(evt feishu.MessageReceiveEvent) {
		log.Printf("[Feishu Webhook] 📩 Inbound msg from user %s (chat %s): %s", evt.Sender.SenderID.OpenID, evt.Message.ChatID, evt.Message.Content)
	})
	mux.Handle("/webhook/feishu", feishuWebhookHandler)

	log.Println("==================================================================")
	log.Printf("🚀 Starting KeyAgent Unified Daemon for [%s] on :%s", agentName, port)
	log.Printf("   - Gitea Webhook URL:  http://localhost:%s/webhook/gitea", port)
	log.Printf("   - Feishu Webhook URL: http://localhost:%s/webhook/feishu", port)
	log.Printf("   - Matrix User:       %s (%s)", matrixUserID, matrixServer)
	log.Printf("   - Mastodon Server:   %s", mastodonServer)
	log.Printf("   - Authentik SSO:     %s", authentikURL)
	log.Printf("   - Vault Secret Path: %s", vaultAddr)
	log.Println("==================================================================")

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatalf("Daemon stopped unexpectedly: %v", err)
	}
}

type SystemMetrics struct {
	LoadAvg    string
	MemUsed    string
	MemTotal   string
	MemPercent string
}

func getSystemMetrics() SystemMetrics {
	metrics := SystemMetrics{
		LoadAvg:    "N/A",
		MemUsed:    "N/A",
		MemTotal:   "N/A",
		MemPercent: "N/A",
	}

	// 1. Load average from /proc/loadavg
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(data))
		if len(fields) >= 3 {
			metrics.LoadAvg = fmt.Sprintf("%s, %s, %s", fields[0], fields[1], fields[2])
		}
	}

	// 2. Memory from /proc/meminfo
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		var totalKB, availKB float64
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %f kB", &totalKB)
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %f kB", &availKB)
			}
		}
		if totalKB > 0 {
			usedKB := totalKB - availKB
			pct := (usedKB / totalKB) * 100
			metrics.MemTotal = fmt.Sprintf("%.0fMB", totalKB/1024)
			metrics.MemUsed = fmt.Sprintf("%.0fMB", usedKB/1024)
			metrics.MemPercent = fmt.Sprintf("%.1f%%", pct)
		}
	}

	return metrics
}
