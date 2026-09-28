// Copyright 2026 Google LLC
//
// Feishu/Lark Channel adapter: inbound webhook (event subscription) plus
// outbound IM send, mirroring the Matrix adapter in matrix.go.

package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"google.golang.org/adk/v2/feishu"
)

// FeishuConfig holds Feishu/Lark app and webhook configuration.
type FeishuConfig struct {
	AppID             string // Feishu app ID
	AppSecret         string // Feishu app secret (for tenant access token)
	VerificationToken string // app Verification Token (url_verification)
	EncryptKey        string // app Encrypt Key (signature + body decryption)
	WebhookAddr       string // listen address, e.g. ":8080"; empty disables inbound
	CallbackPath      string // callback route, e.g. "/feishu/callback"; defaults to it
}

// FeishuClient is the Feishu/Lark unified communications client.
type FeishuClient struct {
	cfg    FeishuConfig
	api    *feishu.Client
	server *http.Server
}

// NewFeishuClient initializes a new FeishuClient.
func NewFeishuClient(cfg FeishuConfig) *FeishuClient {
	return &FeishuClient{
		cfg: cfg,
		api: feishu.New(feishu.Config{AppID: cfg.AppID, AppSecret: cfg.AppSecret}),
	}
}

// SendMessage sends an outbound Feishu text message and returns the message ID.
// receiveIDType is one of open_id | user_id | chat_id | union_id.
func (c *FeishuClient) SendMessage(ctx context.Context, receiveID, receiveIDType, text string) (string, error) {
	content, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return "", fmt.Errorf("feishu: marshal content: %w", err)
	}
	return c.api.SendMessage(ctx, receiveID, receiveIDType, "text", string(content))
}

// OnMessage is invoked for each inbound Feishu message. reply sends a text
// message back to the same sender (open_id).
type OnMessage func(ctx context.Context, evt feishu.MessageReceiveEvent, reply func(text string) error)

// StartWebhook starts the HTTP server that receives Feishu events and invokes
// onMessage for each inbound message. It returns when the server stops. When
// WebhookAddr is empty, inbound is disabled and the call returns immediately.
func (c *FeishuClient) StartWebhook(ctx context.Context, onMessage OnMessage) error {
	if c.cfg.WebhookAddr == "" {
		log.Printf("[Feishu Channel] ⚠️ Inbound webhook disabled: WebhookAddr empty")
		return nil
	}
	path := c.cfg.CallbackPath
	if path == "" {
		path = "/feishu/callback"
	}

	handler := feishu.Webhook(feishu.WebhookConfig{
		VerificationToken: c.cfg.VerificationToken,
		EncryptKey:        c.cfg.EncryptKey,
	}, func(evt feishu.MessageReceiveEvent) {
		reply := func(text string) error {
			rid := evt.Message.OpenID
			if rid == "" {
				rid = evt.Message.ChatID
			}
			if rid == "" {
				return fmt.Errorf("feishu: no receive id to reply to")
			}
			_, err := c.SendMessage(ctx, rid, "open_id", text)
			return err
		}
		if onMessage != nil {
			onMessage(ctx, evt, reply)
		}
	})

	mux := http.NewServeMux()
	mux.Handle(path, handler)
	c.server = &http.Server{
		Addr:              c.cfg.WebhookAddr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		<-ctx.Done()
		_ = c.server.Close()
	}()

	log.Printf("[Feishu Channel] 🔄 Starting Feishu webhook on %s%s", c.cfg.WebhookAddr, path)
	if err := c.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("feishu: webhook server: %w", err)
	}
	return nil
}
