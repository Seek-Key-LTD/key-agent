// Package oracle implements memory.Service backed by Oracle ADB (lake5).
//
// Architecture:
//   1. Classify the session content into one of 3 layers
//   2. Embed the text (384-dim float32)
//   3. Encrypt (deep layer) or leave plaintext (shared)
//   4. Write to PICO_MEMORY_DEEP or PICO_MEMORY_SHARED
//   5. Search via Oracle VECTOR_DISTANCE
//
// Pure Go — uses sijms/go-ora/v2, no ODPI-C / CGO required.
package oracle

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	go_ora "github.com/sijms/go-ora/v2"

	"google.golang.org/adk/v2/internal/utils"
	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/memory/classifier"
	"google.golang.org/adk/v2/session"
)

// OracleMemory implements memory.Service using Oracle ADB.
type OracleMemory struct {
	dsn        string
	secret     []byte // AES-256 key
	classifier *classifier.RuleSet
	embedder   Embedder
	db         *sql.DB
}

// Embedder converts text to a 384-dim float32 vector.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// NewOracleMemory creates a new Oracle-backed memory service.
// dsn is a go-ora connection string, e.g.
//   oracle://admin:pass@host:1522/service?TRACE_FILE=...
func NewOracleMemory(dsn, secretHex string) *OracleMemory {
	m := &OracleMemory{
		dsn:        dsn,
		classifier: classifier.DefaultRuleSet(),
	}
	if secretHex != "" {
		key := make([]byte, 32)
		for i := 0; i+1 < len(secretHex) && i/2 < 32; i += 2 {
			var b byte
			fmt.Sscanf(secretHex[i:i+2], "%x", &b)
			key[i/2] = b
		}
		m.secret = key
	}
	return m
}

// Connect opens the Oracle connection pool. Call once before use.
func (o *OracleMemory) Connect(ctx context.Context) error {
	db, err := sql.Open("oracle", o.dsn)
	if err != nil {
		return fmt.Errorf("oracle open: %w", err)
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(2)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return fmt.Errorf("oracle ping: %w", err)
	}
	o.db = db
	return nil
}

// Close releases the connection pool.
func (o *OracleMemory) Close() error {
	if o.db != nil {
		return o.db.Close()
	}
	return nil
}

// AddSessionToMemory ingests a session into Oracle memory.
func (o *OracleMemory) AddSessionToMemory(ctx context.Context, s session.Session) error {
	if o.db == nil {
		return fmt.Errorf("not connected: call Connect() first")
	}
	var contents []string
	for event := range s.Events().All() {
		if event.Content != nil {
			parts := utils.TextParts(event.Content)
			contents = append(contents, parts...)
		}
	}
	content := joinContents(contents)
	if content == "" {
		return nil
	}
	agentName := s.UserID()

	decision := o.classifier.Classify(agentName, content, nil)
	embedding, err := o.embed(ctx, content)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}
	vec, err := go_ora.NewVector(embedding)
	if err != nil {
		return fmt.Errorf("vector: %w", err)
	}

	if decision.Layer == classifier.LayerShared {
		return o.writeShared(ctx, agentName, content, vec, decision)
	}
	return o.writeDeep(ctx, agentName, content, vec, decision)
}

// SearchMemory searches memory via Oracle VECTOR_DISTANCE.
func (o *OracleMemory) SearchMemory(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	if o.db == nil {
		return nil, fmt.Errorf("not connected")
	}
	embedding, err := o.embed(ctx, req.Query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	vec, err := go_ora.NewVector(embedding)
	if err != nil {
		return nil, fmt.Errorf("vector: %w", err)
	}

	rows, err := o.db.QueryContext(ctx, `
		SELECT agent_name, content
	FROM PICO_MEMORY_SHARED
	WHERE :2 IS NULL OR agent_name = :3
	ORDER BY VECTOR_DISTANCE(embedding, :1, DOT)
	FETCH FIRST 10 ROWS ONLY`, vec, sql.NullString{String: req.UserID, Valid: req.UserID != ""}, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("search shared: %w", err)
	}
	defer rows.Close()

	var entries []memory.Entry
	for rows.Next() {
		var agent, content string
		if err := rows.Scan(&agent, &content); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		entries = append(entries, memory.Entry{
			Author: agent,
		})
	}
	if entries == nil {
		entries = []memory.Entry{}
	}
	return &memory.SearchResponse{Memories: entries}, nil
}

// writeDeep encrypts content and writes to PICO_MEMORY_DEEP.
func (o *OracleMemory) writeDeep(ctx context.Context, agentName, content string, vec *go_ora.Vector, decision *classifier.Decision) error {
	if len(o.secret) < 32 {
		return fmt.Errorf("deep layer requires 32-byte secret key")
	}
	ct, iv, salt := encryptAES256GCM(o.secret, []byte(content))
	meta, _ := json.Marshal(map[string]any{
		"layer":      decision.Layer,
		"agent_name": agentName,
		"confidence": decision.Confidence,
		"reason":     decision.Reason,
		"timestamp":  time.Now().Format(time.RFC3339),
	})
	_, err := o.db.ExecContext(ctx, `
		INSERT INTO PICO_MEMORY_DEEP
		    (agent_name, content_enc, iv, salt, embedding, layer, confidence, reason)
		VALUES (:1, :2, :3, :4, :5, :6, :7, :8)`,
		agentName, ct, iv, salt, vec,
		string(decision.Layer), decision.Confidence, string(meta))
	return err
}

// writeShared writes plaintext content to PICO_MEMORY_SHARED.
func (o *OracleMemory) writeShared(ctx context.Context, agentName, content string, vec *go_ora.Vector, decision *classifier.Decision) error {
	meta, _ := json.Marshal(map[string]any{
		"layer":      decision.Layer,
		"agent_name": agentName,
		"confidence": decision.Confidence,
		"reason":     decision.Reason,
		"timestamp":  time.Now().Format(time.RFC3339),
	})
	_, err := o.db.ExecContext(ctx, `
		INSERT INTO PICO_MEMORY_SHARED
		    (agent_name, content, embedding, layer, confidence, reason)
		VALUES (:1, :2, :3, :4, :5, :6)`,
		agentName, content, vec,
		string(decision.Layer), decision.Confidence, string(meta))
	return err
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
	// Deterministic placeholder embedding (hash-based).
	// Real deployments must set an Embedder.
	return hashEmbed(text), nil
}

// hashEmbed creates a deterministic 384-dim vector from text.
// Not semantically meaningful, but stable for testing.
func hashEmbed(text string) []float32 {
	v := make([]float32, 384)
	for i := range v {
		v[i] = float32(((i*31 + len(text)) % 7) - 3) / 10.0
	}
	// incorporate text characters
	for i, c := range text {
		idx := (i + int(c)) % 384
		v[idx] += float32(c) / 1000.0
	}
	// normalize
	var norm float64
	for _, x := range v {
		norm += float64(x) * float64(x)
	}
	if norm > 0 {
		n := float32(1.0 / math.Sqrt(norm))
		for i := range v {
			v[i] *= n
		}
	}
	return v
}

func floatsToRaw(vals []float32) []byte {
	buf := make([]byte, len(vals)*4)
	for i, v := range vals {
		bits := uint32(math.Float32bits(v))
		buf[i*4] = byte(bits >> 24)
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
