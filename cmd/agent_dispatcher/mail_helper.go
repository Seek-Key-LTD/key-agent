// Copyright 2026 Google LLC
//
// Mail and OTP helper for KeyAgent Guest nodes
// Reads inbox safely via lark-cli with strict 5-second timeout and regex extraction

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

type OTPResult struct {
	Code      string `json:"otp"`
	From      string `json:"from"`
	Subject   string `json:"subject"`
	Date      string `json:"date"`
	MessageID string `json:"message_id"`
}

type LarkMailItem struct {
	Date      string `json:"date"`
	Folder    string `json:"folder"`
	From      string `json:"from"`
	Labels    string `json:"labels"`
	MailboxID string `json:"mailbox_id"`
	MessageID string `json:"message_id"`
	Subject   string `json:"subject"`
	ThreadID  string `json:"thread_id"`
}

type LarkMailList struct {
	Count    int            `json:"count"`
	HasMore  bool           `json:"has_more"`
	Messages []LarkMailItem `json:"messages"`
}

var (
	reLabeledOTP = regexp.MustCompile(`(?i)(?:验证码|code|otp)[:：\s]*([0-9]{3}[- ]?[0-9]{3}|[0-9]{4,8})`)
	reDashedOTP  = regexp.MustCompile(`\b([0-9]{3}-[0-9]{3})\b`)
	reSixDigits  = regexp.MustCompile(`\b([0-9]{6})\b`)
)

func findLarkCLI() string {
	for _, p := range []string{"/usr/local/bin/lark-cli", "/usr/bin/lark-cli"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("lark-cli"); err == nil {
		return p
	}
	return "lark-cli"
}

// GetLatestOTP reads recent emails via lark-cli and extracts OTP codes with a 5s hard timeout
func GetLatestOTP(ctx context.Context) (*OTPResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	larkPath := findLarkCLI()
	cmd := exec.CommandContext(ctx, larkPath, "mail", "+triage", "--format", "json")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("lark-cli mail triage failed: %w (stderr: %s)", err, stderr.String())
	}

	raw := stdout.String()
	// Extract JSON portion between first '{' and last '}'
	startIdx := strings.Index(raw, "{")
	endIdx := strings.LastIndex(raw, "}")
	if startIdx == -1 || endIdx == -1 || endIdx <= startIdx {
		return nil, fmt.Errorf("no valid JSON output from lark-cli triage: %s", raw)
	}

	var mailList LarkMailList
	if err := json.Unmarshal([]byte(raw[startIdx:endIdx+1]), &mailList); err != nil {
		return nil, fmt.Errorf("failed to parse lark mail JSON: %w", err)
	}

	if len(mailList.Messages) == 0 {
		return nil, fmt.Errorf("inbox is empty")
	}

	// 1. First pass: look for OTP in message subjects
	for _, msg := range mailList.Messages {
		if code := extractOTPCode(msg.Subject); code != "" {
			return &OTPResult{
				Code:      code,
				From:      msg.From,
				Subject:   msg.Subject,
				Date:      msg.Date,
				MessageID: msg.MessageID,
			}, nil
		}
	}

	// 2. Second pass: if subject mentions code/verification/whatsapp, fetch body of the first matched message
	for _, msg := range mailList.Messages {
		subLower := strings.ToLower(msg.Subject)
		fromLower := strings.ToLower(msg.From)
		if strings.Contains(subLower, "code") || strings.Contains(subLower, "验证") ||
			strings.Contains(subLower, "whatsapp") || strings.Contains(fromLower, "whatsapp") ||
			strings.Contains(fromLower, "verify") || strings.Contains(fromLower, "security") {

			body, err := fetchMailBody(ctx, larkPath, msg.MessageID)
			if err == nil {
				if code := extractOTPCode(body); code != "" {
					return &OTPResult{
						Code:      code,
						From:      msg.From,
						Subject:   msg.Subject,
						Date:      msg.Date,
						MessageID: msg.MessageID,
					}, nil
				}
			}
			break
		}
	}

	return nil, fmt.Errorf("no OTP code found in recent %d messages", len(mailList.Messages))
}

func extractOTPCode(text string) string {
	if m := reLabeledOTP.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if m := reDashedOTP.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	if m := reSixDigits.FindStringSubmatch(text); len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func fetchMailBody(ctx context.Context, larkPath, messageID string) (string, error) {
	if messageID == "" {
		return "", fmt.Errorf("empty message id")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, larkPath, "mail", "+message", "--message-id", messageID)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[Mail Helper] ⚠️ Could not fetch message %s body: %v", messageID, err)
		return "", err
	}
	return string(out), nil
}
