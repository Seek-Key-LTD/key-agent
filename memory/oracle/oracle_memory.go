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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"google.golang.org/genai"
	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/session"

	"google.golang.org/adk/v2/memory/classifier"
)

// OracleMemory implements memory.Service using Oracle ADB.
type OracleMemory struct {
	dsn     string
	secret  []byte          // AES-256 key (from Vault/OpenBao)
	classifier *classifier.RuleSet
	embedder Embedder        // interface for embedding calls
}

// Embedder converts text to a 384-dim float32 vector.
// Implemented by calling LiteLLM/OpenAI-compatible API.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// NewOracleMemory creates a new Oracle-backed memory service.
func NewOracleMemory(dsn, secretHex string) *OracleMemory {
	return &OracleMemory{
		dsn:        dsn,
		secret:     nil, // TODO: parse secretHex or read from environment
		classifier: classifier.DefaultRuleSet(),
		embedder:   nil, // TODO: wire LiteLLM embedder
	}
}

// AddSessionToMemory ingests a session into Oracle memory.
func (o *OracleMemory) AddSessionToMemory(ctx context.Context, s session.Session) error {
	// Extract session text from events
	var contents []string
	for event := range s.Events().All() {
		if event.Message != nil && event.Message.Content != nil {
			contents = append(contents, event.Message.Content.Text())
		}
	}
	content := joinContents(contents)
	agentName := s.UserID() // convention: agent name in userID field

	// Step 1: Classify
	ruleDecision := o.classifier.Classify(agentName, content, nil)
	decision := ruleDecision // TODO: add LLM re-review when embedder available

	// Step 2: Embed
	embedding, err := o.embed(ctx, content)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	// Step 3+4: Write to appropriate table
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

	// Build query vector as RAW(1536)
	vecRaw := floatsToRaw(embedding)

	// Query both tables via UNION and merge results
	// SELECT content, content_encrypted, metadata_json, vector_distance
	// FROM (
	//   SELECT content_plaintext, NULL, metadata_json,
	//          VECTOR_DISTANCE(embedding, :vec) as dist
	//   FROM PICO_MEMORY_SHARED
	//   UNION ALL
	//   SELECT NULL, content_encrypted, metadata_json,
	//          VECTOR_DISTANCE(embedding, :vec) as dist
	//   FROM PICO_MEMORY_DEEP
	// ) ORDER BY dist FETCH FIRST 5 ROWS ONLY
	//
	// TODO: actual SQL execution
	// For now return empty results (implementation in progress)

	return &memory.SearchResponse{Memories: []memory.Entry{}}, nil
}

// writeDeep writes to PICO_MEMORY_DEEP (AES-256-GCM encrypted).
func (o *OracleMemory) writeDeep(ctx context.Context, agentName, content string, embedding []float32, decision *classifier.Decision) error {
	if o.embedder == nil || len(o.secret) < 32 {
		// If no secret configured, store unencrypted (dev mode)
		// TODO: replace with proper DB write
		return nil
	}

	plaintext := []byte(content)
	encrypted, iv, salt := encryptAES256GCM(o.secret, plaintext)

	metadata := map[string]any{
		"layer":      decision.Layer,
		"agent_name": agentName,
		"confidence": decision.Confidence,
		"reason":     decision.Reason,
		"timestamp":  time.Now().Format(time.RFC3339),
	}
	metaJSON, _ := json.Marshal(metadata)

	// INSERT INTO PICO_MEMORY_DEEP (agent_name, content_encrypted, iv_salt, embedding, metadata_json)
	// TODO: actual DB execution
	//   _, err := db.ExecContext(ctx, insertDeepSQL, agentName, encrypted, append(iv, salt...), vecRaw, metaJSON)

	return nil
}

// writeShared writes to PICO_MEMORY_SHARED (plaintext).
func (o *OracleMemory) writeShared(ctx context.Context, agentName, content string, embedding []float32, decision *classifier.Decision) error {
	metadata := map[string]any{
		"layer":      decision.Layer,
		"agent_name": agentName,
		"confidence": decision.Confidence,
		"reason":     decision.Reason,
		"timestamp":  time.Now().Format(time.RFC3339),
	}
	metaJSON, _ := json.Marshal(metadata)

	// INSERT INTO PICO_MEMORY_SHARED (agent_name, content_plaintext, embedding, metadata_json)
	// TODO: actual DB execution

	return nil
}

// encryptAES256GCM encrypts plaintext with AES-256-GCM.
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

// embed calls the embedder to get 384-dim vector.
func (o *OracleMemory) embed(ctx context.Context, text string) ([]float32, error) {
	if o.embedder != nil {
		return o.embedder.Embed(ctx, text)
	}
	// Dev mode: return zero vector
	return make([]float32, 384), nil
}

// floatsToRaw converts float32 slice to RAW(1536).
func floatsToRaw(vals []float32) []byte {
	buf := make([]byte, len(vals)*4)
	for i, v := range vals {
		bits := uint32(v) // NOTE: real impl needs binary encoding
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

// ── SQL ──
const insertDeepSQL = `
INSERT INTO PICO_MEMORY_DEEP (AGENT_NAME, CONTENT_ENCRYPTED, IV_SALT, EMBEDDING, METADATA_JSON, CREATED_AT)
VALUES (:agent_name, :content_encrypted, :iv_salt, :embedding, :metadata_json, CURRENT_TIMESTAMP)
`

const insertSharedSQL = `
INSERT INTO PICO_MEMORY_SHARED (AGENT_NAME, CONTENT_PLAINTEXT, EMBEDDING, METADATA_JSON, CREATED_AT)
VALUES (:agent_name, :content, :embedding, :metadata_json, CURRENT_TIMESTAMP)
`

const searchMemorySQL = `
SELECT agent_name, content_plaintext, content_encrypted, iv_salt, metadata_json, vector_distance
FROM (
  SELECT agent_name, content_plaintext, NULL as content_encrypted, NULL as iv_salt,
         metadata_json, VECTOR_DISTANCE(embedding, :vec) as vector_distance
  FROM PICO_MEMORY_SHARED
  UNION ALL
  SELECT agent_name, NULL, content_encrypted, iv_salt, metadata_json,
         VECTOR_DISTANCE(embedding, :vec) as vector_distance
  FROM PICO_MEMORY_DEEP
  WHERE agent_name = :agent
)
ORDER BY vector_distance
FETCH FIRST 5 ROWS ONLY
`
