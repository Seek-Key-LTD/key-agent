// Copyright 2026 Google LLC
//
// Package feishu (event) handles Feishu/Lark inbound event callbacks: request
// signature verification, optional AES-256-CBC decryption of the event body,
// URL verification (challenge) response, and an HTTP handler that dispatches
// inbound messages to a callback. No external dependencies.

package feishu

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

// EventEnvelope is the common envelope of a Feishu/Lark event callback.
type EventEnvelope struct {
	Schema    string          `json:"schema"`
	Header    EventHeader     `json:"header"`
	Event     json.RawMessage `json:"event"`
	Encrypt   string          `json:"encrypt"` // present when body encryption is enabled
	Type      string          `json:"type"`    // "url_verification" | "event_callback"
	Token     string          `json:"token"`   // url_verification token (optional check)
	Challenge string          `json:"challenge"`
}

// EventHeader is the event envelope header.
type EventHeader struct {
	EventID    string `json:"event_id"`
	EventType  string `json:"event_type"` // e.g. "im.message.receive_v1"
	AppID      string `json:"app_id"`
	TenantKey  string `json:"tenant_key"`
	CreateTime string `json:"create_time"`
}

// MessageReceiveEvent is the payload of an im.message.receive_v1 event.
type MessageReceiveEvent struct {
	Sender struct {
		SenderID struct {
			OpenID string `json:"open_id"`
			UserID string `json:"user_id"`
		} `json:"sender_id"`
	} `json:"sender"`
	Message struct {
		MessageID      string `json:"message_id"`
		ConversationID string `json:"conversation_id"`
		ChatID         string `json:"chat_id"`
		ChatType       string `json:"chat_type"` // "p2p" | "group"
		Content        string `json:"content"`   // JSON string, e.g. {"text":"hi"}
		MsgType        string `json:"msg_type"`
		OpenID         string `json:"open_id"`
	} `json:"message"`
}

// VerifySignature verifies an X-Lark-Signature header value. The string to sign
// is timestamp+nonce+body, HMAC-SHA256 with key, then base64-encoded.
// key is the app's Encrypt Key (or Verification Token).
func VerifySignature(key, ts, nonce string, body []byte, signature string) bool {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(ts + nonce + string(body)))
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

// decrypt decrypts a Feishu AES-256-CBC encrypted event body. The key is the
// app's Encrypt Key (32 bytes; a 43-char base64 string is accepted). IV is the
// first 16 bytes of the key. Padding is PKCS#7.
func decrypt(encryptKey, ciphertextB64 string) ([]byte, error) {
	key := []byte(encryptKey)
	if len(key) != aes.BlockSize*2 { // 32 bytes for AES-256
		if dec, err := base64.StdEncoding.DecodeString(encryptKey); err == nil && len(dec) == 32 {
			key = dec
		} else {
			return nil, fmt.Errorf("feishu: encrypt key must be 32 bytes, got %d", len(key))
		}
	}
	data, err := base64.StdEncoding.DecodeString(ciphertextB64)
	if err != nil {
		return nil, fmt.Errorf("feishu: decode ciphertext: %w", err)
	}
	if len(data) < aes.BlockSize {
		return nil, errors.New("feishu: ciphertext too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("feishu: new cipher: %w", err)
	}
	mode := cipher.NewCBCDecrypter(block, key[:aes.BlockSize])
	plain := make([]byte, len(data))
	mode.CryptBlocks(plain, data)
	return pkcs7Unpad(plain)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, errors.New("feishu: empty plaintext")
	}
	pad := int(data[len(data)-1])
	if pad < 1 || pad > aes.BlockSize || pad > len(data) {
		return nil, errors.New("feishu: invalid padding")
	}
	for _, b := range data[len(data)-pad:] {
		if int(b) != pad {
			return nil, errors.New("feishu: invalid padding")
		}
	}
	return data[:len(data)-pad], nil
}

// ParseEnvelope reads a raw callback body (possibly encrypted) and returns the
// envelope. Provide encryptKey when the app has body encryption enabled.
func ParseEnvelope(body []byte, encryptKey string) (*EventEnvelope, error) {
	var probe struct {
		Encrypt string `json:"encrypt"`
	}
	if err := json.Unmarshal(body, &probe); err == nil && probe.Encrypt != "" {
		if encryptKey == "" {
			return nil, errors.New("feishu: event body is encrypted but no encrypt key configured")
		}
		plain, err := decrypt(encryptKey, probe.Encrypt)
		if err != nil {
			return nil, fmt.Errorf("feishu: decrypt event: %w", err)
		}
		body = plain
	}
	var env EventEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("feishu: parse envelope: %w", err)
	}
	return &env, nil
}

// UnmarshalMessage decodes an im.message.receive_v1 event payload.
func UnmarshalMessage(raw json.RawMessage) (MessageReceiveEvent, error) {
	var evt MessageReceiveEvent
	if err := json.Unmarshal(raw, &evt); err != nil {
		return evt, fmt.Errorf("feishu: parse message event: %w", err)
	}
	return evt, nil
}

// WebhookConfig configures the Feishu event webhook.
type WebhookConfig struct {
	// VerificationToken is the app's Verification Token; checked on
	// url_verification when non-empty.
	VerificationToken string
	// EncryptKey is the app's Encrypt Key, used for signature verification and
	// for decrypting the event body when encryption is enabled.
	EncryptKey string
}

// Webhook returns an http.Handler that verifies, decrypts and parses Feishu
// events, answers the URL verification challenge, and forwards inbound messages
// to onMessage. The handler always responds 200 to event callbacks so Feishu
// does not retry indefinitely.
func Webhook(cfg WebhookConfig, onMessage func(MessageReceiveEvent)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read body", http.StatusBadRequest)
			return
		}
		env, err := ParseEnvelope(body, cfg.EncryptKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// URL verification challenge.
		if env.Type == "url_verification" {
			if cfg.VerificationToken != "" && env.Token != cfg.VerificationToken {
				http.Error(w, "token mismatch", http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"challenge": env.Challenge})
			return
		}

		// Signature verification (skip when no key configured, e.g. local dev).
		if cfg.EncryptKey != "" {
			sig := r.Header.Get("X-Lark-Signature")
			ts := r.Header.Get("X-Lark-Request-Timestamp")
			nonce := r.Header.Get("X-Lark-Request-Nonce")
			if !VerifySignature(cfg.EncryptKey, ts, nonce, body, sig) {
				http.Error(w, "signature mismatch", http.StatusForbidden)
				return
			}
		}

		w.WriteHeader(http.StatusOK)

		if env.Header.EventType == "im.message.receive_v1" && onMessage != nil {
			if evt, err := UnmarshalMessage(env.Event); err == nil {
				onMessage(evt)
			}
		}
	})
}
