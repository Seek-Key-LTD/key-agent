// Copyright 2026 Google LLC
//
// Mastodon ActivityPub Channel Adapter
// Supports Account Verification, Status Posting (Toots), and Mention/Notification Listening

package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// MastodonConfig holds configuration for connecting to a Mastodon instance
type MastodonConfig struct {
	ServerURL   string // e.g. "https://mastodon.capitaltrain.cn"
	AccessToken string // OAuth2 Bearer Access Token
	AgentName   string // e.g. "ruby", "topaz"
}

// MastodonAccount represents an actor account profile on Mastodon
type MastodonAccount struct {
	ID          string `json:"id"`
	Username    string `json:"username"`
	Acct        string `json:"acct"`
	DisplayName string `json:"display_name"`
	URL         string `json:"url"`
	Bot         bool   `json:"bot"`
}

// MastodonStatus represents a status/toot on Mastodon
type MastodonStatus struct {
	ID        string           `json:"id"`
	CreatedAt string           `json:"created_at"`
	Content   string           `json:"content"`
	Account   *MastodonAccount `json:"account"`
	InReplyTo *string          `json:"in_reply_to_id"`
	URL       string           `json:"url"`
}

// MastodonNotification represents an activity notification (e.g. mention, follow)
type MastodonNotification struct {
	ID        string           `json:"id"`
	Type      string           `json:"type"` // "mention", "status", "reblog", "follow"
	CreatedAt string           `json:"created_at"`
	Account   *MastodonAccount `json:"account"`
	Status    *MastodonStatus  `json:"status"`
}

// MastodonClient provides ActivityPub / Mastodon communication capabilities
type MastodonClient struct {
	cfg        MastodonConfig
	httpClient *http.Client
}

// NewMastodonClient initializes a new Mastodon client
func NewMastodonClient(cfg MastodonConfig) *MastodonClient {
	if cfg.ServerURL == "" {
		cfg.ServerURL = "https://mastodon.capitaltrain.cn"
	}
	cfg.ServerURL = strings.TrimRight(cfg.ServerURL, "/")
	return &MastodonClient{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 15 * time.Second},
	}
}

// VerifyCredentials verifies that the access token is valid and returns the agent account
func (c *MastodonClient) VerifyCredentials(ctx context.Context) (*MastodonAccount, error) {
	if c.cfg.AccessToken == "" {
		return nil, fmt.Errorf("mastodon access_token is empty")
	}

	reqURL := c.cfg.ServerURL + "/api/v1/accounts/verify_credentials"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("verify credentials failed (status %d): %s", resp.StatusCode, string(b))
	}

	var acc MastodonAccount
	if err := json.NewDecoder(resp.Body).Decode(&acc); err != nil {
		return nil, err
	}
	return &acc, nil
}

// PostStatus posts a new toot/status update to the Mastodon server
func (c *MastodonClient) PostStatus(ctx context.Context, status string, inReplyToID string, visibility string) (*MastodonStatus, error) {
	if c.cfg.AccessToken == "" {
		return nil, fmt.Errorf("mastodon access_token is empty")
	}
	if visibility == "" {
		visibility = "public"
	}

	form := url.Values{}
	form.Set("status", status)
	form.Set("visibility", visibility)
	if inReplyToID != "" {
		form.Set("in_reply_to_id", inReplyToID)
	}

	reqURL := c.cfg.ServerURL + "/api/v1/statuses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("post status failed (status %d): %s", resp.StatusCode, string(b))
	}

	var st MastodonStatus
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, err
	}
	return &st, nil
}

// GetNotifications queries recent notifications (e.g. mentions)
func (c *MastodonClient) GetNotifications(ctx context.Context, sinceID string, types []string) ([]MastodonNotification, error) {
	if c.cfg.AccessToken == "" {
		return nil, fmt.Errorf("mastodon access_token is empty")
	}

	q := url.Values{}
	if sinceID != "" {
		q.Set("since_id", sinceID)
	}
	for _, t := range types {
		q.Add("types[]", t)
	}
	q.Set("limit", "20")

	reqURL := fmt.Sprintf("%s/api/v1/notifications?%s", c.cfg.ServerURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get notifications failed (status %d): %s", resp.StatusCode, string(b))
	}

	var notes []MastodonNotification
	if err := json.NewDecoder(resp.Body).Decode(&notes); err != nil {
		return nil, err
	}
	return notes, nil
}

// StartNotificationLoop periodically polls for new mentions and invokes callback
func (c *MastodonClient) StartNotificationLoop(ctx context.Context, interval time.Duration, onMention func(n *MastodonNotification)) {
	if c.cfg.AccessToken == "" {
		log.Printf("[Mastodon Channel] ⚠️ Notification loop disabled: access_token missing")
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	var sinceID string
	initNotes, err := c.GetNotifications(ctx, "", []string{"mention"})
	if err == nil && len(initNotes) > 0 {
		sinceID = initNotes[0].ID
		log.Printf("[Mastodon Channel] 🦣 Initialized latest notification ID: %s", sinceID)
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("[Mastodon Channel] Notification loop stopped")
			return
		case <-ticker.C:
			notes, err := c.GetNotifications(ctx, sinceID, []string{"mention"})
			if err != nil {
				log.Printf("[Mastodon Channel] ⚠️ Error polling mentions: %v", err)
				continue
			}
			for i := len(notes) - 1; i >= 0; i-- {
				n := notes[i]
				sinceID = n.ID
				if onMention != nil {
					onMention(&n)
				}
			}
		}
	}
}
