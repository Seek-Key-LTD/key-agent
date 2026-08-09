// Copyright 2026 Google LLC
//
// Package feishu provides a direct Feishu Open API client with no external dependencies.

package feishu

import (
	"context"
	"encoding/json"
	"fmt"
)

// Mailbox represents a Feishu mail mailbox.
type Mailbox struct {
	MailboxID string `json:"mailbox_id"`
	Email     string `json:"email"`
	Name      string `json:"name"`
}

// ListMailboxes lists mailboxes visible to the app.
func (c *Client) ListMailboxes(ctx context.Context) ([]Mailbox, error) {
	raw, err := c.Do(ctx, "GET", "/open-apis/mail/v1/mailboxes", nil)
	if err != nil {
		return nil, fmt.Errorf("feishu: list mailboxes: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			Items []Mailbox `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("feishu: parse list mailboxes response: %w", err)
	}
	if resp.Code != 0 {
		return nil, fmt.Errorf("feishu: list mailboxes api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.Items, nil
}

// mailRecipient is a recipient of a mail message.
type mailRecipient struct {
	Email string `json:"email"`
}

// SendMail sends an email from the given mailbox.
func (c *Client) SendMail(ctx context.Context, mailboxID string, to []string, subject, body string) (string, error) {
	recipients := make([]mailRecipient, 0, len(to))
	for _, addr := range to {
		recipients = append(recipients, mailRecipient{Email: addr})
	}
	payload := map[string]interface{}{
		"subject": subject,
		"body":    body,
		"to":      recipients,
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("feishu: marshal send mail: %w", err)
	}

	path := fmt.Sprintf("/open-apis/mail/v1/mailboxes/%s/messages", mailboxID)
	raw, err := c.Do(ctx, "POST", path, rawBody)
	if err != nil {
		return "", fmt.Errorf("feishu: send mail: %w", err)
	}

	var resp struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
		Data struct {
			MessageID string `json:"message_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return "", fmt.Errorf("feishu: parse send mail response: %w", err)
	}
	if resp.Code != 0 {
		return "", fmt.Errorf("feishu: send mail api error code=%d msg=%s", resp.Code, resp.Msg)
	}

	return resp.Data.MessageID, nil
}
