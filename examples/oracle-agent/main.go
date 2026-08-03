// oracle-agent demonstrates a minimal ADK-Go agent using Oracle memory backend.
//
// Usage:
//   export OPENAI_BASE_URL=https://litellm.capitaltrain.cn/v1
//   export OPENAI_API_KEY=your-key
//   go run ./examples/oracle-agent
package main

import (
	"flag"
	"log"
	"os"

	agent2 "google.golang.org/adk/v2/agent"
	"google.golang.org/adk/v2/memory"
	"google.golang.org/adk/v2/model/openai"

	_ "google.golang.org/adk/v2/memory/classifier"
	_ "google.golang.org/adk/v2/memory/oracle"
	_ "google.golang.org/adk/v2/session/oracle"
)

func main() {
	var (
		dsn         = flag.String("dsn", os.Getenv("ORACLE_DSN"), "Oracle DSN")
		agentName   = flag.String("agent", "oracle-agent", "Agent name")
		modelID     = flag.String("model", "nova-deepseek-v4-flash", "Model ID")
		baseURL     = flag.String("base-url", os.Getenv("OPENAI_BASE_URL"), "LLM base URL")
		apiKey      = flag.String("api-key", os.Getenv("OPENAI_API_KEY"), "LLM API key")
		memoryKey   = flag.String("memory-key", os.Getenv("ORACLE_MEMORY_KEY"), "AES-256 key hex for memory encryption")
		secret      = flag.String("secret", os.Getenv("LITELLM_API_KEY"), "LiteLLM API key")
	)
	flag.Parse()

	if *dsn == "" {
		log.Fatal("-dsn required (or set ORACLE_DSN)")
	}
	if *baseURL == "" {
		log.Fatal("-base-url required (or set OPENAI_BASE_URL)")
	}
	if *secret == "" {
		log.Fatal("-secret required (or set LITELLM_API_KEY)")
	}

	// ── LLM Provider ──
	llm, err := openai.NewProvider(openai.Config{
		BaseURL:       *baseURL,
		APIKey:        *secret,
		Model:         *modelID,
	})
	if err != nil {
		log.Fatalf("create LLM provider: %v", err)
	}

	// ── Memory Store ──
	mem := memory.NewOracleMemory(*dsn, *memoryKey)

	// ── Session Store ──
	sess := session.NewOracleSession(*dsn)

	// ── Agent ──
	agent := agent2.NewAgent(agent2.Config{
		Name:      *agentName,
		Model:     llm,
		SessionSvc: sess,
		MemorySvc: mem,
	})

	log.Printf("Agent %q running with Oracle memory + session backend", *agentName)
	log.Printf("LLM: %s@%s", *modelID, *baseURL)
	log.Printf("Oracle DSN: %s", maskDSN(*dsn))
	// TODO: start agent runner / web server
}
}

func maskDSN(dsn string) string {
	// TODO: mask credentials in dsn for logging
	return dsn
}
