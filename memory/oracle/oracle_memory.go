// Package oracle implements memory.Service backed by Oracle ADB (lake5).
//
// Architecture:
//   1. Classify the session content into one of 3 layers
//   2. Embed the text (384-dim float32)
//   3. Encrypt (deep layer) or leave plaintext (shared)
//   4. Write to PICO_MEMORY_DEEP or PICO_MEMORY_SHARED
//   5. Search via Oracle VECTOR_DISTANCE
package oracle

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/memory/classifier"
	"google.golang.org/adk/v2/session"

	"google.golang.org/adk/v2/internal/utils"
)

// OracleMemory implements memory.Service using Oracle ADB.
type OracleMemory struct {
	dsn         string
	secret      []byte                // AES-256 key (from Vault/OpenBao)
	classifier  *classifier.RuleSet
	embedder    Embedder              // interface for embedding calls
}

// Embedder converts text to a 384-dim float32 vector.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// NewOracleMemory creates a new Oracle-backed memory service.
func NewOracleMemory(dsn, secretHex string) *OracleMemory {
	return &OracleMemory{
		dsn:        dsn,
		secret:     nil,
		classifier: classifier.DefaultRuleSet(),
		embedder:   nil,
	}
}

// AddSessionToMemory ingests a session into Oracle memory.
func (o *OracleMemory) AddSessionToMemory(ctx context.Context, s session.Session) error {
	_ = ctx
	var contents []string
	for event := range s.Events().All() {
		if event.Content != nil {
			parts := utils.TextParts(event.Content)
			contents = append(contents, parts...)
		}
	}
	content := joinContents(contents)
	agentName := s.UserID()

	ruleDecision := o.classifier.Classify(agentName, content, nil)
	decision := ruleDecision

	embedding, err := o.embed(ctx, content)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	if decision.Layer == classifier.LayerShared {
		return o.writeShared(ctx, agentName, content, embedding, decision)
	}
	return o.writeDeep(ctx, agentName, content, embedding, decision)
}

// SearchMemory searches memory via Oracle VECTOR_DISTANCE.
func (o *OracleMemory) SearchMemory(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	if o.embedder == nil {
		return &memory.SearchResponse{Memories: []memory.Entry{}}, nil
	}
	embedding, err := o.embed(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	_ = embedding
	// TODO: actual SQL execution via go-ora
	return &memory.SearchResponse{Memories: []memory.Entry{}}, nil
}

func (o *OracleMemory) writeDeep(ctx context.Context, agentName, content string, embedding []float32, decision *classifier.Decision) error {
	_ = ctx
	_ = embedding
	if o.embedder == nil || len(o.secret) < 32 {
		return nil
	}
	plaintext := []byte(content)
	_, _, _ = encryptAES256GCM(o.secret, plaintext)

	_, _ = json.Marshal(map[string]any{
		"layer":      decision.Layer,
		"agent_name": agentName,
		"confidence": decision.Confidence,
		"reason":     decision.Reason,
		"timestamp":  time.Now().Format(time.RFC3339),
	})
	return nil
}

func (o *OracleMemory) writeShared(ctx context.Context, agentName, content string, embedding []float32, decision *classifier.Decision) error {
	_ = ctx
	_ = embedding
	_, _ = json.Marshal(map[string]any{
		"layer":      decision.Layer,
		"agent_name": agentName,
		"confidence": decision.Confidence,
		"reason":     decision.Reason,
		"timestamp":  time.Now().Format(time.RFC3339),
	})
	return nil
}

func encryptAES256GCM(key, plaintext []byte) (ciphertext, iv, salt []byte) {
	block, _ := aes.NewCipher(key)
	aesgcm, _ := cipher.NewGCM(block)

	iv = make([]byte, aesgcm.NonceSize())
	io.ReadFull(rand.Reader, iv)

	salt = make([]byte, 12)
	io.ReadFull(rand.Reader, salt)

	ciphertext = aesgcm.Seal(nil, iv, plaintext, nil)
	return ciphertext, iv, salt
}

func (o *OracleMemory) embed(ctx context.Context, text string) ([]float32, error) {
	if o.embedder != nil {
		return o.embedder.Embed(ctx, text)
	}
	return make([]float32, 384), nil
}

func floatsToRaw(vals []float32) []byte {
	buf := make([]byte, len(vals)*4)
	for i, v := range vals {
		bits := uint32(math.Float32bits(v))
		buf[i*4]   = byte(bits >> 24)
		buf[i*4+1] = byte(bits >> 16)
		buf[i*4+2] = byte(bits >> 8)
		buf[i*4+3] = byte(bits)
	}
	return buf
}

func joinContents(cs []string) string {
	if len(cs) == 0 {
		return ""
	}
	out := make([]byte, 0, 4096)
	for i, c := range cs {
		if i > 0 {
			out = append(out, '\n', '\n')
		}
		out = append(out, c...)
	}
	return string(out)
}
